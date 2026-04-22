package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildSearchURLEscapesQuery(t *testing.T) {
	url := buildSearchURL("golang tips & tricks")

	if !strings.HasPrefix(url, SearchEndpointURL()+"?q=") {
		t.Fatalf("expected search endpoint prefix, got %q", url)
	}
	if strings.Contains(url, "tips & tricks") {
		t.Fatalf("expected query to be escaped, got %q", url)
	}
	if !strings.Contains(url, "tips+%26+tricks") {
		t.Fatalf("expected escaped query string, got %q", url)
	}
}

func TestSearchAndFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"title":"Result","url":"https://example.com","body":"Demo body"}]`))
		case "/page":
			_, _ = w.Write([]byte("page body"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalEndpoint := searchEndpoint
	searchEndpoint = server.URL + "/search"
	t.Cleanup(func() {
		searchEndpoint = originalEndpoint
	})

	results, err := Search(context.Background(), "demo query", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Result" {
		t.Fatalf("unexpected search results: %#v", results)
	}

	content, err := Fetch(context.Background(), server.URL+"/page")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if content != "page body" {
		t.Fatalf("unexpected fetched content: %q", content)
	}
}

func TestSearchAndFetchErrorPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	originalEndpoint := searchEndpoint
	searchEndpoint = server.URL
	t.Cleanup(func() {
		searchEndpoint = originalEndpoint
	})

	if _, err := Search(context.Background(), "demo", 5); err == nil {
		t.Fatal("expected Search to fail on invalid JSON response")
	}
	if _, err := Fetch(context.Background(), "://bad"); err == nil {
		t.Fatal("expected Fetch to fail on invalid URL")
	}
}
