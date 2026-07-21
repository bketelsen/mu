package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	githubAPIVersion = "2022-11-28"
	userAgent        = "Mu/1.0"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	token      func() string
}

func NewClient(httpClient *http.Client, baseURL string, token func() string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{httpClient: httpClient, baseURL: strings.TrimRight(baseURL, "/"), token: token}
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values, dst any) (PageInfo, error) {
	token := ""
	if c.token != nil {
		token = strings.TrimSpace(c.token())
	}
	if token == "" {
		return PageInfo{}, &APIError{Kind: ErrorNotConfigured}
	}

	requestURL, err := url.Parse(c.baseURL + endpoint)
	if err != nil {
		return PageInfo{}, &APIError{Kind: ErrorInvalid}
	}
	requestURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return PageInfo{}, &APIError{Kind: ErrorInvalid}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PageInfo{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return PageInfo{}, err
	}
	if len(body) > maxBodyBytes {
		return PageInfo{}, &APIError{Kind: ErrorUpstream, Status: resp.StatusCode}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return PageInfo{}, responseError(resp)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return PageInfo{}, &APIError{Kind: ErrorUpstream, Status: resp.StatusCode}
	}

	page := parseLinks(resp.Header.Get("Link"))
	page.Page = queryInt(query, "page", 1)
	page.PerPage = queryInt(query, "per_page", defaultPerPage)
	return page, nil
}

func responseError(resp *http.Response) error {
	apiErr := &APIError{Status: resp.StatusCode}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		apiErr.Kind = ErrorUnauthorized
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			apiErr.Kind = ErrorRateLimited
			apiErr.Reset = unixHeader(resp.Header.Get("X-RateLimit-Reset"))
		} else {
			apiErr.Kind = ErrorForbidden
		}
	case http.StatusNotFound:
		apiErr.Kind = ErrorNotFound
	case http.StatusTooManyRequests:
		apiErr.Kind = ErrorRateLimited
		apiErr.retryAfter = retryAfter(resp.Header.Get("Retry-After"))
	default:
		apiErr.Kind = ErrorUpstream
	}
	return apiErr
}

func parseLinks(header string) PageInfo {
	var page PageInfo
	for _, link := range strings.Split(header, ",") {
		parts := strings.Split(link, ";")
		if len(parts) < 2 {
			continue
		}
		target := strings.Trim(strings.TrimSpace(parts[0]), "<>")
		u, err := url.Parse(target)
		if err != nil {
			continue
		}
		number := queryInt(u.Query(), "page", 0)
		for _, parameter := range parts[1:] {
			rel := strings.TrimSpace(parameter)
			if !strings.HasPrefix(rel, "rel=") {
				continue
			}
			switch strings.Trim(strings.TrimPrefix(rel, "rel="), `"`) {
			case "next":
				page.Next = number
			case "prev":
				page.Prev = number
			case "first":
				page.First = number
			case "last":
				page.Last = number
			}
		}
	}
	return page
}

func queryInt(values url.Values, key string, fallback int) int {
	number, err := strconv.Atoi(values.Get(key))
	if err != nil || number < 1 {
		return fallback
	}
	return number
}

func unixHeader(value string) time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}

func retryAfter(value string) time.Duration {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
