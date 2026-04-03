package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type browserSessionContextKey struct{}

func WithBrowserSession(ctx context.Context, sessionID string) context.Context {
	if strings.TrimSpace(sessionID) == "" {
		return ctx
	}
	return context.WithValue(ctx, browserSessionContextKey{}, strings.TrimSpace(sessionID))
}

func browserSessionFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(browserSessionContextKey{}).(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func resolveBrowserSessionID(ctx context.Context, input map[string]any) string {
	if input != nil {
		if sessionID, _ := input["session_id"].(string); strings.TrimSpace(sessionID) != "" {
			return strings.TrimSpace(sessionID)
		}
	}
	if sessionID := browserSessionFromContext(ctx); sessionID != "" {
		if input != nil {
			input["session_id"] = sessionID
		}
		return sessionID
	}
	if input != nil {
		input["session_id"] = "default"
	}
	return "default"
}

type browserSession struct {
	id        string
	allocCtx  context.Context
	rootCtx   context.Context
	cancel    context.CancelFunc
	started   time.Time
	pages     map[string]*browserPage
	activeTab string
}

type browserPage struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	created time.Time
	lastURL string
}

var (
	browserMu       sync.Mutex
	browserSessions = map[string]*browserSession{}
)

func getBrowserSession(sessionID string) (*browserSession, error) {
	browserMu.Lock()
	defer browserMu.Unlock()
	if sessionID == "" {
		sessionID = "default"
	}
	if existing, ok := browserSessions[sessionID]; ok {
		return existing, nil
	}
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Headless, chromedp.DisableGPU)...)
	rootCtx, cancel := chromedp.NewContext(allocCtx)
	bs := &browserSession{id: sessionID, allocCtx: allocCtx, rootCtx: rootCtx, cancel: cancel, started: time.Now().UTC(), pages: map[string]*browserPage{}}
	page, err := bs.newPage("tab-1")
	if err != nil {
		cancel()
		return nil, err
	}
	bs.activeTab = page.id
	browserSessions[sessionID] = bs
	return bs, nil
}

func (s *browserSession) newPage(tabID string) (*browserPage, error) {
	if strings.TrimSpace(tabID) == "" {
		tabID = fmt.Sprintf("tab-%d", len(s.pages)+1)
	}
	ctx, cancel := chromedp.NewContext(s.rootCtx)
	page := &browserPage{id: tabID, ctx: ctx, cancel: cancel, created: time.Now().UTC()}
	s.pages[tabID] = page
	return page, nil
}

func (s *browserSession) ensurePage(tabID string) (*browserPage, error) {
	if strings.TrimSpace(tabID) == "" {
		tabID = s.activeTab
	}
	if page, ok := s.pages[tabID]; ok {
		return page, nil
	}
	page, err := s.newPage(tabID)
	if err != nil {
		return nil, err
	}
	s.activeTab = page.id
	return page, nil
}

func resolveBrowserTabID(input map[string]any) string {
	if input == nil {
		return ""
	}
	tabID, _ := input["tab_id"].(string)
	return strings.TrimSpace(tabID)
}

func getBrowserPage(sessionID string, tabID string) (*browserSession, *browserPage, error) {
	bs, err := getBrowserSession(sessionID)
	if err != nil {
		return nil, nil, err
	}
	browserMu.Lock()
	defer browserMu.Unlock()
	page, err := bs.ensurePage(tabID)
	if err != nil {
		return nil, nil, err
	}
	if page != nil {
		bs.activeTab = page.id
	}
	return bs, page, nil
}

func BrowserNavigateTool(ctx context.Context, input map[string]any) (string, error) {
	urlStr, _ := input["url"].(string)
	sessionID := resolveBrowserSessionID(ctx, input)
	if strings.TrimSpace(urlStr) == "" {
		return "", fmt.Errorf("url is required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	var title string
	err = chromedp.Run(pageCtx.ctx,
		chromedp.Navigate(urlStr),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Title(&title),
	)
	if err != nil {
		return "", err
	}
	pageCtx.lastURL = urlStr
	return fmt.Sprintf("Navigated tab %s to %s (title: %s)", pageCtx.id, urlStr, title), nil
}

func BrowserClickTool(ctx context.Context, input map[string]any) (string, error) {
	selector, _ := input["selector"].(string)
	sessionID := resolveBrowserSessionID(ctx, input)
	if strings.TrimSpace(selector) == "" {
		return "", fmt.Errorf("selector is required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	err = chromedp.Run(pageCtx.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Clicked %s", selector), nil
}

func BrowserTypeTool(ctx context.Context, input map[string]any) (string, error) {
	selector, _ := input["selector"].(string)
	text, _ := input["text"].(string)
	sessionID := resolveBrowserSessionID(ctx, input)
	if strings.TrimSpace(selector) == "" {
		return "", fmt.Errorf("selector is required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	err = chromedp.Run(pageCtx.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.SetValue(selector, "", chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Typed into %s", selector), nil
}

func BrowserScreenshotTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	path, _ := input["path"].(string)
	selector, _ := input["selector"].(string)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	var buf []byte
	if strings.TrimSpace(selector) != "" {
		err = chromedp.Run(pageCtx.ctx, chromedp.Screenshot(selector, &buf, chromedp.NodeVisible, chromedp.ByQuery))
	} else {
		err = chromedp.Run(pageCtx.ctx, chromedp.FullScreenshot(&buf, 90))
	}
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Saved screenshot to %s", path), nil
}

func BrowserScreenshotToolWithPolicy(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	path, _ := input["path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	resolved := normalizePolicyArtifactPath(path, opts.WorkingDir)
	if opts.Policy != nil {
		if err := opts.Policy.CheckWritePath(resolved); err != nil {
			return "", err
		}
	} else if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	cloned := cloneBrowserInput(input)
	cloned["path"] = resolved
	return BrowserScreenshotTool(ctx, cloned)
}

func BrowserUploadTool(ctx context.Context, input map[string]any) (string, error) {
	selector, _ := input["selector"].(string)
	path, _ := input["path"].(string)
	sessionID := resolveBrowserSessionID(ctx, input)
	if strings.TrimSpace(selector) == "" || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("selector and path are required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	err = chromedp.Run(pageCtx.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.SetUploadFiles(selector, []string{absPath}, chromedp.ByQuery),
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Uploaded %s via %s", absPath, selector), nil
}

func BrowserUploadToolWithPolicy(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	selector, _ := input["selector"].(string)
	path, _ := input["path"].(string)
	sessionID := resolveBrowserSessionID(ctx, input)
	if strings.TrimSpace(selector) == "" || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("selector and path are required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	resolved := normalizePolicyArtifactPath(path, opts.WorkingDir)
	if opts.Policy != nil {
		if err := opts.Policy.CheckBrowserUpload(resolved, pageCtx.lastURL); err != nil {
			return "", err
		}
	} else if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	cloned := cloneBrowserInput(input)
	cloned["path"] = resolved
	return BrowserUploadTool(ctx, cloned)
}

func BrowserEvaluateTool(ctx context.Context, input map[string]any) (string, error) {
	expression, _ := input["expression"].(string)
	sessionID := resolveBrowserSessionID(ctx, input)
	if strings.TrimSpace(expression) == "" {
		return "", fmt.Errorf("expression is required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	var result any
	err = chromedp.Run(pageCtx.ctx, chromedp.Evaluate(expression, &result))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", result), nil
}

func BrowserSnapshotTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	var html string
	var title string
	var currentURL string
	err = chromedp.Run(pageCtx.ctx,
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	)
	if err != nil {
		return "", err
	}
	if len(html) > 8000 {
		html = html[:8000] + "..."
	}
	return fmt.Sprintf("Tab: %s\nURL: %s\nTitle: %s\nHTML:\n%s", pageCtx.id, currentURL, title, html), nil
}

func BrowserCloseTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	if sessionID == "" {
		sessionID = "default"
	}
	browserMu.Lock()
	defer browserMu.Unlock()
	bs, ok := browserSessions[sessionID]
	if !ok {
		return "Browser session already closed", nil
	}
	for _, page := range bs.pages {
		page.cancel()
	}
	bs.cancel()
	delete(browserSessions, sessionID)
	return fmt.Sprintf("Closed browser session %s", sessionID), nil
}

func BrowserPDFTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	path, _ := input["path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	var pdf []byte
	err = chromedp.Run(pageCtx.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		buf, _, err := page.PrintToPDF().WithPrintBackground(true).Do(ctx)
		if err != nil {
			return err
		}
		pdf = buf
		return nil
	}))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Saved PDF to %s", path), nil
}

func BrowserPDFToolWithPolicy(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	path, _ := input["path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	resolved := normalizePolicyArtifactPath(path, opts.WorkingDir)
	if opts.Policy != nil {
		if err := opts.Policy.CheckWritePath(resolved); err != nil {
			return "", err
		}
	} else if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	cloned := cloneBrowserInput(input)
	cloned["path"] = resolved
	return BrowserPDFTool(ctx, cloned)
}

func BrowserScreenshotBase64Tool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	var buf []byte
	err = chromedp.Run(pageCtx.ctx, chromedp.FullScreenshot(&buf, 90))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func BrowserWaitTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	selector, _ := input["selector"].(string)
	state, _ := input["state"].(string)
	timeoutMs, _ := input["timeout_ms"].(float64)
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	if timeoutMs <= 0 {
		timeoutMs = 10000
	}
	waitCtx, cancel := context.WithTimeout(pageCtx.ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	if strings.TrimSpace(selector) == "" {
		selector = "body"
	}
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		state = "visible"
	}
	var action chromedp.Action
	switch state {
	case "ready":
		action = chromedp.WaitReady(selector, chromedp.ByQuery)
	case "enabled":
		action = chromedp.WaitEnabled(selector, chromedp.ByQuery)
	default:
		action = chromedp.WaitVisible(selector, chromedp.ByQuery)
	}
	if err := chromedp.Run(waitCtx, action); err != nil {
		return "", err
	}
	return fmt.Sprintf("Waited for %s to become %s", selector, state), nil
}

func BrowserSelectTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	selector, _ := input["selector"].(string)
	value, _ := input["value"].(string)
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(selector) == "" || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("selector and value are required")
	}
	if err := chromedp.Run(pageCtx.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.SetValue(selector, value, chromedp.ByQuery),
	); err != nil {
		return "", err
	}
	return fmt.Sprintf("Selected %s on %s", value, selector), nil
}

func BrowserPressTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	selector, _ := input["selector"].(string)
	key, _ := input["key"].(string)
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("key is required")
	}
	actions := []chromedp.Action{}
	if strings.TrimSpace(selector) != "" {
		actions = append(actions, chromedp.WaitVisible(selector, chromedp.ByQuery), chromedp.Focus(selector, chromedp.ByQuery))
	}
	actions = append(actions, chromedp.KeyEvent(key))
	if err := chromedp.Run(pageCtx.ctx, actions...); err != nil {
		return "", err
	}
	return fmt.Sprintf("Pressed %s", key), nil
}

func BrowserScrollTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	selector, _ := input["selector"].(string)
	direction, _ := input["direction"].(string)
	pixels, _ := input["pixels"].(float64)
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	if pixels == 0 {
		pixels = 600
	}
	if strings.EqualFold(strings.TrimSpace(direction), "up") {
		pixels = -pixels
	}
	var script string
	if strings.TrimSpace(selector) != "" {
		script = fmt.Sprintf(`(() => { const el = document.querySelector(%q); if (!el) return "missing"; el.scrollBy(0, %d); return "ok"; })()`, selector, int(pixels))
	} else {
		script = fmt.Sprintf(`(() => { window.scrollBy(0, %d); return "ok"; })()`, int(pixels))
	}
	var result string
	if err := chromedp.Run(pageCtx.ctx, chromedp.Evaluate(script, &result)); err != nil {
		return "", err
	}
	if result == "missing" {
		return "", fmt.Errorf("selector not found: %s", selector)
	}
	return fmt.Sprintf("Scrolled by %d pixels", int(pixels)), nil
}

func BrowserDownloadTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	selector, _ := input["selector"].(string)
	urlStr, _ := input["url"].(string)
	path, _ := input["path"].(string)
	_, pageCtx, err := getBrowserPage(sessionID, resolveBrowserTabID(input))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.TrimSpace(urlStr) == "" && strings.TrimSpace(selector) != "" {
		if err := chromedp.Run(pageCtx.ctx, chromedp.AttributeValue(selector, "href", &urlStr, nil, chromedp.ByQuery)); err != nil || strings.TrimSpace(urlStr) == "" {
			_ = chromedp.Run(pageCtx.ctx, chromedp.AttributeValue(selector, "src", &urlStr, nil, chromedp.ByQuery))
		}
	}
	if strings.TrimSpace(urlStr) == "" {
		return "", fmt.Errorf("url or selector with href/src is required")
	}
	var base64Content string
	fetchJS := fmt.Sprintf(`(async () => { const res = await fetch(%q, {credentials: 'include'}); const buf = await res.arrayBuffer(); let binary=''; const bytes = new Uint8Array(buf); const chunk = 0x8000; for (let i = 0; i < bytes.length; i += chunk) { binary += String.fromCharCode(...bytes.slice(i, i+chunk)); } return btoa(binary); })()`, urlStr)
	if err := chromedp.Run(pageCtx.ctx, chromedp.Evaluate(fetchJS, &base64Content)); err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Downloaded %s to %s", urlStr, path), nil
}

func BrowserDownloadToolWithPolicy(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	path, _ := input["path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	resolved := normalizePolicyArtifactPath(path, opts.WorkingDir)
	if opts.Policy != nil {
		if err := opts.Policy.CheckWritePath(resolved); err != nil {
			return "", err
		}
	} else if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	cloned := cloneBrowserInput(input)
	cloned["path"] = resolved
	return BrowserDownloadTool(ctx, cloned)
}

func BrowserTabNewTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	tabID, _ := input["tab_id"].(string)
	urlStr, _ := input["url"].(string)
	bs, err := getBrowserSession(sessionID)
	if err != nil {
		return "", err
	}
	browserMu.Lock()
	page, err := bs.newPage(strings.TrimSpace(tabID))
	if err == nil {
		bs.activeTab = page.id
	}
	browserMu.Unlock()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(urlStr) != "" {
		_, err = BrowserNavigateTool(ctx, map[string]any{"session_id": sessionID, "tab_id": page.id, "url": urlStr})
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Created tab %s in browser session %s", page.id, sessionID), nil
}

func BrowserTabListTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	bs, err := getBrowserSession(sessionID)
	if err != nil {
		return "", err
	}
	browserMu.Lock()
	defer browserMu.Unlock()
	rows := make([]string, 0, len(bs.pages))
	for id, page := range bs.pages {
		marker := " "
		if id == bs.activeTab {
			marker = "*"
		}
		rows = append(rows, fmt.Sprintf("%s %s %s", marker, id, strings.TrimSpace(page.lastURL)))
	}
	if len(rows) == 0 {
		return "No tabs", nil
	}
	return strings.Join(rows, "\n"), nil
}

func BrowserTabSwitchTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	tabID, _ := input["tab_id"].(string)
	if strings.TrimSpace(tabID) == "" {
		return "", fmt.Errorf("tab_id is required")
	}
	bs, err := getBrowserSession(sessionID)
	if err != nil {
		return "", err
	}
	browserMu.Lock()
	defer browserMu.Unlock()
	if _, ok := bs.pages[tabID]; !ok {
		return "", fmt.Errorf("tab not found: %s", tabID)
	}
	bs.activeTab = tabID
	return fmt.Sprintf("Switched to tab %s", tabID), nil
}

func BrowserTabCloseTool(ctx context.Context, input map[string]any) (string, error) {
	sessionID := resolveBrowserSessionID(ctx, input)
	tabID, _ := input["tab_id"].(string)
	if strings.TrimSpace(tabID) == "" {
		return "", fmt.Errorf("tab_id is required")
	}
	bs, err := getBrowserSession(sessionID)
	if err != nil {
		return "", err
	}
	browserMu.Lock()
	defer browserMu.Unlock()
	page, ok := bs.pages[tabID]
	if !ok {
		return "", fmt.Errorf("tab not found: %s", tabID)
	}
	page.cancel()
	delete(bs.pages, tabID)
	if bs.activeTab == tabID {
		bs.activeTab = ""
		for id := range bs.pages {
			bs.activeTab = id
			break
		}
		if bs.activeTab == "" {
			newPage, err := bs.newPage("tab-1")
			if err == nil {
				bs.activeTab = newPage.id
			}
		}
	}
	return fmt.Sprintf("Closed tab %s", tabID), nil
}

func cloneBrowserInput(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
