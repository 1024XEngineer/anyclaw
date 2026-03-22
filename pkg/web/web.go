package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

func Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 20 {
		maxResults = 20
	}

	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnyClaw/1.0)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var ddgResp struct {
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
		AbstractText string `json:"AbstractText"`
		AbstractURL  string `json:"AbstractURL"`
		Heading      string `json:"Heading"`
	}

	if err := json.Unmarshal(body, &ddgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var results []SearchResult

	if ddgResp.AbstractText != "" {
		results = append(results, SearchResult{
			Title:       ddgResp.Heading,
			URL:         ddgResp.AbstractURL,
			Description: ddgResp.AbstractText,
		})
	}

	for _, topic := range ddgResp.RelatedTopics {
		if topic.FirstURL == "" || topic.Text == "" {
			continue
		}
		if len(results) >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:       extractTitle(topic.Text),
			URL:         topic.FirstURL,
			Description: topic.Text,
		})
	}

	return results, nil
}

func Fetch(ctx context.Context, targetURL string) (string, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnyClaw/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return "", fmt.Errorf("not an HTML page (Content-Type: %s)", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	html := string(body)
	text := stripHTML(html)
	text = strings.TrimSpace(text)

	if len(text) > 8000 {
		text = text[:8000] + "\n\n[Content truncated...]"
	}

	return text, nil
}

func extractTitle(text string) string {
	if idx := strings.Index(text, " - "); idx > 0 {
		return text[:idx]
	}
	if idx := strings.Index(text, " — "); idx > 0 {
		return text[:idx]
	}
	if len(text) > 100 {
		return text[:100] + "..."
	}
	return text
}

func stripHTML(html string) string {
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			result.WriteByte(' ')
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}

	text := result.String()
	text = strings.Join(strings.Fields(text), " ")

	return text
}
