package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientGetHeadersAndPagination(t *testing.T) {
	var token = "first-token"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Method; got != http.MethodGet {
			t.Fatalf("method = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Fatalf("Accept = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubAPIVersion {
			t.Fatalf("X-GitHub-Api-Version = %q", got)
		}
		w.Header().Set("Link", `<https://api.github.com/resource?page=3>; rel="next", <https://api.github.com/resource?page=1>; rel="prev"`)
		_, _ = io.WriteString(w, `[{"id":1}]`)
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return token })
	var got []struct {
		ID int64 `json:"id"`
	}
	page, err := c.get(context.Background(), "/resource", nil, &got)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || page.Next != 3 || page.Prev != 1 {
		t.Fatalf("got %#v, page %#v", got, page)
	}

	token = "rotated-token"
	if _, err := c.get(context.Background(), "/resource", nil, &got); err != nil {
		t.Fatal(err)
	}
}

func TestClientGetErrors(t *testing.T) {
	reset := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name    string
		token   string
		status  int
		headers map[string]string
		kind    ErrorKind
		reset   time.Time
	}{
		{name: "no token", kind: ErrorNotConfigured},
		{name: "unauthorized", token: "token", status: http.StatusUnauthorized, kind: ErrorUnauthorized},
		{name: "rate limited forbidden", token: "token", status: http.StatusForbidden, headers: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": fmt.Sprint(reset.Unix())}, kind: ErrorRateLimited, reset: reset},
		{name: "forbidden", token: "token", status: http.StatusForbidden, kind: ErrorForbidden},
		{name: "not found", token: "token", status: http.StatusNotFound, kind: ErrorNotFound},
		{name: "rate limited", token: "token", status: http.StatusTooManyRequests, kind: ErrorRateLimited},
		{name: "upstream", token: "token", status: http.StatusInternalServerError, kind: ErrorUpstream},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range test.headers {
					w.Header().Set(key, value)
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, "upstream details must not escape")
			}))
			defer ts.Close()

			c := NewClient(ts.Client(), ts.URL, func() string { return test.token })
			_, err := c.get(context.Background(), "/resource", nil, &struct{}{})
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error %v is not an APIError", err)
			}
			if apiErr.Kind != test.kind {
				t.Errorf("kind = %q, want %q", apiErr.Kind, test.kind)
			}
			if !apiErr.Reset.Equal(test.reset) {
				t.Errorf("reset = %v, want %v", apiErr.Reset, test.reset)
			}
			if strings.Contains(err.Error(), "upstream details") {
				t.Errorf("error leaks upstream body: %q", err)
			}
		})
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"`+strings.Repeat("x", maxBodyBytes)+`"`)
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return "token" })
	_, err := c.get(context.Background(), "/resource", nil, &struct{}{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != ErrorUpstream {
		t.Fatalf("error = %v, want upstream APIError", err)
	}
}

func TestClientHonorsCanceledContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer ts.Close()

	c := NewClient(ts.Client(), ts.URL, func() string { return "token" })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.get(ctx, "/resource", nil, &struct{}{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
