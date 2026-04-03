package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/chromedp/chromedp"
)

type NetworkInterceptor struct {
	mu         sync.RWMutex
	enabled    bool
	patterns   []*InterceptPattern
	handlers   map[string]RequestHandler
	requestLog []NetworkRequest
}

type InterceptPattern struct {
	URLPattern string
	Regex      *regexp.Regexp
}

type RequestHandler func(*NetworkRequest) *NetworkResponse

type NetworkRequest struct {
	ID        string
	URL       string
	Method    string
	Headers   map[string]string
	PostData  string
	Timestamp int64
}

type NetworkResponse struct {
	StatusCode int
	Status     string
	Headers    map[string]string
	Body       string
	Base64Body string
	Delay      int
}

func NewNetworkInterceptor() *NetworkInterceptor {
	return &NetworkInterceptor{
		handlers:   make(map[string]RequestHandler),
		requestLog: make([]NetworkRequest, 0),
	}
}

func (ni *NetworkInterceptor) Enable() {
	ni.mu.Lock()
	defer ni.mu.Unlock()
	ni.enabled = true
}

func (ni *NetworkInterceptor) Disable() {
	ni.mu.Lock()
	defer ni.mu.Unlock()
	ni.enabled = false
}

func (ni *NetworkInterceptor) AddPattern(pattern string) error {
	ni.mu.Lock()
	defer ni.mu.Unlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	ni.patterns = append(ni.patterns, &InterceptPattern{
		URLPattern: pattern,
		Regex:      re,
	})
	return nil
}

func (ni *NetworkInterceptor) SetHandler(pattern string, handler RequestHandler) {
	ni.mu.Lock()
	defer ni.mu.Unlock()
	ni.handlers[pattern] = handler
}

func (ni *NetworkInterceptor) ShouldIntercept(url string) bool {
	ni.mu.RLock()
	defer ni.mu.RUnlock()

	if !ni.enabled {
		return false
	}

	for _, p := range ni.patterns {
		if p.Regex.MatchString(url) {
			return true
		}
	}
	return false
}

func (ni *NetworkInterceptor) HandleRequest(req *NetworkRequest) *NetworkResponse {
	ni.mu.RLock()
	defer ni.mu.RUnlock()

	for pattern, handler := range ni.handlers {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(req.URL) {
			return handler(req)
		}
	}
	return nil
}

func (ni *NetworkInterceptor) GetRequestLog() []NetworkRequest {
	ni.mu.RLock()
	defer ni.mu.RUnlock()
	return ni.requestLog
}

func (ni *NetworkInterceptor) ClearLog() {
	ni.mu.Lock()
	defer ni.mu.Unlock()
	ni.requestLog = nil
}

type EnhancedBrowser struct {
	ctx    context.Context
	cancel context.CancelFunc
	ni     *NetworkInterceptor

	headers   map[string]string
	userAgent string
}

func NewEnhancedBrowser(opts *CDPOptions) (*EnhancedBrowser, error) {
	if opts == nil {
		opts = DefaultCDPOptions()
	}

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts.AllocatorOptions()...)
	rootCtx, cancel := chromedp.NewContext(allocCtx)

	eb := &EnhancedBrowser{
		ctx:    rootCtx,
		cancel: cancel,
		ni:     NewNetworkInterceptor(),
	}

	return eb, nil
}

func (eb *EnhancedBrowser) Navigate(url string) error {
	return chromedp.Run(eb.ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	)
}

func (eb *EnhancedBrowser) NavigateWithHeaders(url string) error {
	actions := []chromedp.Action{
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	}

	_ = eb.headers

	return chromedp.Run(eb.ctx, actions...)
}

func (eb *EnhancedBrowser) SetHeader(key, value string) {
	if eb.headers == nil {
		eb.headers = make(map[string]string)
	}
	eb.headers[key] = value
}

func (eb *EnhancedBrowser) SetUserAgent(ua string) {
	eb.userAgent = ua
	eb.SetHeader("User-Agent", ua)
}

func (eb *EnhancedBrowser) ClearHeaders() {
	eb.headers = nil
}

func (eb *EnhancedBrowser) GetLocalStorage(key string) (string, error) {
	var result string
	err := chromedp.Run(eb.ctx,
		chromedp.Evaluate(fmt.Sprintf("localStorage.getItem('%s')", key), &result),
	)
	return result, err
}

func (eb *EnhancedBrowser) SetLocalStorage(key, value string) error {
	return chromedp.Run(eb.ctx,
		chromedp.Evaluate(fmt.Sprintf("localStorage.setItem('%s', '%s')", key, value), nil),
	)
}

func (eb *EnhancedBrowser) RemoveLocalStorage(key string) error {
	return chromedp.Run(eb.ctx,
		chromedp.Evaluate(fmt.Sprintf("localStorage.removeItem('%s')", key), nil),
	)
}

func (eb *EnhancedBrowser) ClearLocalStorage() error {
	return chromedp.Run(eb.ctx,
		chromedp.Evaluate("localStorage.clear()", nil),
	)
}

func (eb *EnhancedBrowser) GetAllLocalStorage() (map[string]string, error) {
	var result string
	err := chromedp.Run(eb.ctx,
		chromedp.Evaluate("JSON.stringify(localStorage)", &result),
	)
	if err != nil {
		return nil, err
	}

	var storage map[string]string
	if err := json.Unmarshal([]byte(result), &storage); err != nil {
		return nil, err
	}
	return storage, nil
}

func (eb *EnhancedBrowser) GetSessionStorage(key string) (string, error) {
	var result string
	err := chromedp.Run(eb.ctx,
		chromedp.Evaluate(fmt.Sprintf("sessionStorage.getItem('%s')", key), &result),
	)
	return result, err
}

func (eb *EnhancedBrowser) SetSessionStorage(key, value string) error {
	return chromedp.Run(eb.ctx,
		chromedp.Evaluate(fmt.Sprintf("sessionStorage.setItem('%s', '%s')", key, value), nil),
	)
}

func (eb *EnhancedBrowser) ClearSessionStorage() error {
	return chromedp.Run(eb.ctx,
		chromedp.Evaluate("sessionStorage.clear()", nil),
	)
}

func (eb *EnhancedBrowser) GetNetworkInterceptor() *NetworkInterceptor {
	return eb.ni
}

func (eb *EnhancedBrowser) BlockURL(pattern string) error {
	return eb.ni.AddPattern(pattern)
}

func (eb *EnhancedBrowser) InterceptAndMock(pattern string, response *NetworkResponse) error {
	if err := eb.ni.AddPattern(pattern); err != nil {
		return err
	}

	eb.ni.SetHandler(pattern, func(req *NetworkRequest) *NetworkResponse {
		return response
	})

	return nil
}

func (eb *EnhancedBrowser) Close() error {
	if eb.cancel != nil {
		eb.cancel()
	}
	return nil
}

type ElementFinder struct {
	ctx context.Context
}

func NewElementFinder(ctx context.Context) *ElementFinder {
	return &ElementFinder{ctx: ctx}
}

func (ef *ElementFinder) FindByText(text string) (string, error) {
	var selector string
	err := chromedp.Run(ef.ctx,
		chromedp.Evaluate(
			fmt.Sprintf(`Array.from(document.querySelectorAll("*")).find(el => el.textContent.includes('%s'))?.tagName`, text),
			&selector,
		),
	)
	return selector, err
}

func (ef *ElementFinder) FindByAttribute(attr, value string) (string, error) {
	return fmt.Sprintf("[%s=\"%s\"]", attr, value), nil
}

func (ef *ElementFinder) FindByXPath(xpath string) (string, error) {
	var result bool
	err := chromedp.Run(ef.ctx,
		chromedp.Evaluate(
			fmt.Sprintf(`document.evaluate('%s', document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue !== null`, xpath),
			&result,
		),
	)
	if err != nil || !result {
		return "", fmt.Errorf("xpath not found")
	}
	return xpath, nil
}

func (ef *ElementFinder) Count(selector string) (int, error) {
	var count int
	err := chromedp.Run(ef.ctx,
		chromedp.Evaluate(
			fmt.Sprintf(`document.querySelectorAll('%s').length`, selector),
			&count,
		),
	)
	return count, err
}

type FormHandler struct {
	ctx context.Context
}

func NewFormHandler(ctx context.Context) *FormHandler {
	return &FormHandler{ctx: ctx}
}

func (fh *FormHandler) Fill(selector string, values map[string]string) error {
	for field, value := range values {
		fieldSelector := fmt.Sprintf("%s [name='%s']", selector, field)
		if err := chromedp.Run(fh.ctx,
			chromedp.SetValue(fieldSelector, value, chromedp.ByQuery),
		); err != nil {
			return err
		}
	}
	return nil
}

func (fh *FormHandler) Submit(selector string) error {
	return chromedp.Run(fh.ctx,
		chromedp.Submit(selector, chromedp.ByQuery),
	)
}

func (fh *FormHandler) Reset(selector string) error {
	return chromedp.Run(fh.ctx,
		chromedp.Reset(selector, chromedp.ByQuery),
	)
}

func (fh *FormHandler) GetValues(selector string, fields []string) (map[string]string, error) {
	result := make(map[string]string)

	for _, field := range fields {
		fieldSelector := fmt.Sprintf("%s [name='%s']", selector, field)
		var value string
		if err := chromedp.Run(fh.ctx,
			chromedp.Value(fieldSelector, &value, chromedp.ByQuery),
		); err != nil {
			return nil, err
		}
		result[field] = value
	}

	return result, nil
}

func ResolveCDPSelector(selector string) string {
	selector = strings.TrimSpace(selector)

	if strings.HasPrefix(selector, "//") {
		return selector
	}

	if strings.HasPrefix(selector, "#") {
		return selector
	}

	if strings.HasPrefix(selector, ".") {
		return selector
	}

	if strings.Contains(selector, "=") {
		parts := strings.SplitN(selector, "=", 2)
		return fmt.Sprintf("[%s=\"%s\"]", parts[0], parts[1])
	}

	return selector
}

func ParseSelector(selector string) (string, string) {
	selector = strings.TrimSpace(selector)

	if strings.HasPrefix(selector, "//") {
		return selector, "xpath"
	}

	if strings.HasPrefix(selector, "#") {
		return selector, "id"
	}

	if strings.HasPrefix(selector, ".") {
		return selector, "class"
	}

	return selector, "css"
}
