package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

var searchEndpoint = "https://ddg-api.vercel.app/search"

type SearchResult struct {
	Title       string
	URL         string
	Description string
}

func Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 5
	}

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", buildSearchURL(query), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"body"`
	}

	if err := json.Unmarshal(body, &results); err != nil {
		return nil, err
	}

	output := make([]SearchResult, 0, len(results))
	for i, r := range results {
		if i >= maxResults {
			break
		}
		output = append(output, SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		})
	}

	return output, nil
}

func SearchEndpointURL() string {
	return searchEndpoint
}

func buildSearchURL(query string) string {
	return searchEndpoint + "?q=" + url.QueryEscape(query)
}

func Fetch(ctx context.Context, urlStr string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
