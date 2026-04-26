package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type Tool func(context.Context, ...chromedp.Action) error

type cleanupFunc func()

var runChromedp = chromedp.Run

func jsStringLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		// json.Marshal on a Go string should never fail; keep a safe fallback.
		return `""`
	}
	return string(encoded)
}

func combineCleanup(funcs ...cleanupFunc) cleanupFunc {
	return func() {
		for _, fn := range funcs {
			if fn != nil {
				fn()
			}
		}
	}
}

type BrowserTool struct {
	ctx     context.Context
	tooltip chromedp.Action
}

func NewBrowserTool(ctx context.Context) (*BrowserTool, error) {
	return &BrowserTool{
		ctx: ctx,
	}, nil
}

func (b *BrowserTool) Navigate(url string) error {
	return runChromedp(b.ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	)
}

func (b *BrowserTool) Screenshot() ([]byte, error) {
	var buf []byte
	err := runChromedp(b.ctx,
		chromedp.FullScreenshot(&buf, 90),
	)
	return buf, err
}

func (b *BrowserTool) Click(selector string) error {
	return runChromedp(b.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
	)
}

func (b *BrowserTool) Type(selector, text string) error {
	return runChromedp(b.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.SetValue(selector, text, chromedp.ByQuery),
	)
}

func (b *BrowserTool) GetElementText(selector string) (string, error) {
	var text string
	err := runChromedp(b.ctx,
		chromedp.Text(selector, &text, chromedp.ByQuery),
	)
	return text, err
}

func (b *BrowserTool) GetElementAttribute(selector, attr string) (string, error) {
	var result string
	err := runChromedp(b.ctx,
		chromedp.AttributeValue(selector, attr, &result, nil, chromedp.ByQuery),
	)
	return result, err
}

func (b *BrowserTool) GetElementHTML(selector string) (string, error) {
	var html string
	err := runChromedp(b.ctx,
		chromedp.InnerHTML(selector, &html, chromedp.ByQuery),
	)
	return html, err
}

func (b *BrowserTool) IsVisible(selector string) (bool, error) {
	var visible bool
	err := runChromedp(b.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
	)
	visible = (err == nil)
	return visible, nil
}

func (b *BrowserTool) WaitForSelector(selector string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	return runChromedp(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
	)
}

func (b *BrowserTool) WaitForNavigation(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	return runChromedp(ctx,
		chromedp.WaitReady("body"),
	)
}

func (b *BrowserTool) Scroll(x, y float64) error {
	expr := fmt.Sprintf("window.scrollTo(%v, %v)", x, y)
	return runChromedp(b.ctx,
		chromedp.Evaluate(expr, nil),
	)
}

func (b *BrowserTool) ScrollToElement(selector string) error {
	expr := fmt.Sprintf("document.querySelector(%s).scrollIntoView()", jsStringLiteral(selector))
	return runChromedp(b.ctx,
		chromedp.Evaluate(expr, nil),
	)
}

func (b *BrowserTool) GetPageSource() (string, error) {
	var source string
	err := runChromedp(b.ctx,
		chromedp.InnerHTML("html", &source),
	)
	return source, err
}

func (b *BrowserTool) GetURL() (string, error) {
	var url string
	err := runChromedp(b.ctx,
		chromedp.Location(&url),
	)
	return url, err
}

func (b *BrowserTool) GetTitle() (string, error) {
	var title string
	err := runChromedp(b.ctx,
		chromedp.Title(&title),
	)
	return title, err
}

func (b *BrowserTool) Close() error {
	return nil
}

func ResolveSelectorBy(selector string, by string) string {
	switch strings.ToLower(by) {
	case "xpath":
		return selector
	case "css", "":
		return selector
	default:
		return selector
	}
}

type CDPOptions struct {
	Headless      bool
	WindowWidth   int
	WindowHeight  int
	UserAgent     string
	Proxy         string
	Incognito     bool
	DisableImages bool
	CacheDisabled bool
}

func DefaultCDPOptions() *CDPOptions {
	return &CDPOptions{
		Headless:     true,
		WindowWidth:  1920,
		WindowHeight: 1080,
	}
}

type allocatorFlagOverride struct {
	name  string
	value any
}

func (o *CDPOptions) allocatorFlagOverrides() []allocatorFlagOverride {
	var overrides []allocatorFlagOverride

	if !o.Headless {
		overrides = append(overrides,
			allocatorFlagOverride{name: "headless", value: false},
			allocatorFlagOverride{name: "hide-scrollbars", value: false},
			allocatorFlagOverride{name: "mute-audio", value: false},
		)
	}

	if o.DisableImages {
		overrides = append(overrides, allocatorFlagOverride{name: "disable-images", value: true})
	}
	if o.CacheDisabled {
		overrides = append(overrides, allocatorFlagOverride{name: "disk-cache-size", value: 0})
	}

	return overrides
}

func (o *CDPOptions) AllocatorOptions() []chromedp.ExecAllocatorOption {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)

	for _, override := range o.allocatorFlagOverrides() {
		opts = append(opts, chromedp.Flag(override.name, override.value))
	}
	if o.WindowWidth > 0 && o.WindowHeight > 0 {
		opts = append(opts, chromedp.WindowSize(o.WindowWidth, o.WindowHeight))
	}
	if o.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(o.UserAgent))
	}
	if o.Proxy != "" {
		opts = append(opts, chromedp.ProxyServer(o.Proxy))
	}

	return opts
}

func newAllocatorContext(opts *CDPOptions) (context.Context, cleanupFunc) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts.AllocatorOptions()...)
	rootCtx, rootCancel := chromedp.NewContext(allocCtx)
	return rootCtx, combineCleanup(
		func() { rootCancel() },
		func() { allocCancel() },
	)
}

func RunInContext(ctx context.Context, opts *CDPOptions, fn func(*BrowserTool) error) error {
	if opts == nil {
		opts = DefaultCDPOptions()
	}

	rootCtx, cancel := newAllocatorContext(opts)
	defer cancel()

	tool, err := NewBrowserTool(rootCtx)
	if err != nil {
		return err
	}

	return fn(tool)
}
