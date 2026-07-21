package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepositoriesListsVisibleRepos(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" || r.URL.Query().Get("page") != "2" ||
			r.URL.Query().Get("per_page") != "30" || r.URL.Query().Get("sort") != "updated" ||
			r.URL.Query().Get("direction") != "desc" ||
			r.URL.Query().Get("affiliation") != "owner,collaborator,organization_member" {
			t.Fatalf("unexpected request %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `[{"id":1,"full_name":"micro/mu"},{"id":2,"full_name":"micro/go-micro"}]`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	got, page, err := c.Repositories(context.Background(), "", 2, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].FullName != "micro/mu" || page.Page != 2 {
		t.Fatalf("got %#v, page %#v", got, page)
	}
}

func TestRepositoriesSearchesWhenQueryPresent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" || r.URL.Query().Get("q") != "mu" {
			t.Fatalf("unexpected request %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `{"items":[{"id":1,"full_name":"micro/mu"}]}`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	got, _, err := c.Repositories(context.Background(), "mu", 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FullName != "micro/mu" {
		t.Fatalf("got %#v", got)
	}
}

func TestRepositoryEscapesAndValidatesCoordinates(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/repos/micro/mu" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"id":1,"full_name":"micro/mu"}`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	if _, err := c.Repository(context.Background(), "micro", "mu"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Repository(context.Background(), "micro/other", "mu"); err == nil {
		t.Fatal("accepted invalid owner")
	}
	if _, err := c.Repository(context.Background(), "micro", ""); err == nil {
		t.Fatal("accepted empty repo")
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d", calls)
	}
}

func TestRepositoriesNormalizesPageBounds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("per_page") != "30" {
			t.Fatalf("unexpected request %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	_, page, err := c.Repositories(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PerPage != 30 {
		t.Fatalf("page = %#v", page)
	}
}

func TestRepositoriesCapsPerPage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "100" {
			t.Fatalf("unexpected request %s", r.URL.String())
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	defer ts.Close()
	c := NewClient(ts.Client(), ts.URL, func() string { return "test-token" })
	_, page, err := c.Repositories(context.Background(), "", 1, 200)
	if err != nil {
		t.Fatal(err)
	}
	if page.PerPage != 100 {
		t.Fatalf("per page = %d", page.PerPage)
	}
}

func TestRepositoriesRejectsLongQuery(t *testing.T) {
	c := NewClient(nil, "https://api.github.com", func() string { return "test-token" })
	_, _, err := c.Repositories(context.Background(), strings.Repeat("x", 257), 1, 30)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Kind != ErrorInvalid {
		t.Fatalf("error = %v, want invalid APIError", err)
	}
}
