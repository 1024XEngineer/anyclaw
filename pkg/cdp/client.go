package cdp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type Tool func(context.Context, ...chromedp.Action) error

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
	return chromedp.Run(b.ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	)
}

func (b *BrowserTool) Screenshot() ([]byte, error) {
	var buf []byte
	err := chromedp.Run(b.ctx,
		chromedp.FullScreenshot(&buf, 90),
	)
	return buf, err
}

func (b *BrowserTool) Click(selector string) error {
	return chromedp.Run(b.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
	)
}

func (b *BrowserTool) Type(selector, text string) error {
	return chromedp.Run(b.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.SetValue(selector, text, chromedp.ByQuery),
	)
}

func (b *BrowserTool) GetElementText(selector string) (string, error) {
	var text string
	err := chromedp.Run(b.ctx,
		chromedp.Text(selector, &text, chromedp.ByQuery),
	)
	return text, err
}

func (b *BrowserTool) GetElementAttribute(selector, attr string) (string, error) {
	var result string
	err := chromedp.Run(b.ctx,
		chromedp.AttributeValue(selector, attr, &result, nil, chromedp.ByQuery),
	)
	return result, err
}

func (b *BrowserTool) GetElementHTML(selector string) (string, error) {
	var html string
	err := chromedp.Run(b.ctx,
		chromedp.InnerHTML(selector, &html, chromedp.ByQuery),
	)
	return html, err
}

func (b *BrowserTool) IsVisible(selector string) (bool, error) {
	var visible bool
	err := chromedp.Run(b.ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
	)
	visible = (err == nil)
	return visible, nil
}

func (b *BrowserTool) WaitForSelector(selector string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	return chromedp.Run(ctx,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
	)
}

func (b *BrowserTool) WaitForNavigation(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	return chromedp.Run(ctx,
		chromedp.WaitReady("body"),
	)
}

func (b *BrowserTool) Scroll(x, y float64) error {
	expr := fmt.Sprintf("window.scrollTo(%v, %v)", x, y)
	return chromedp.Run(b.ctx,
		chromedp.Evaluate(expr, nil),
	)
}

func (b *BrowserTool) ScrollToElement(selector string) error {
	expr := fmt.Sprintf(`document.querySelector('%s').scrollIntoView()`, selector)
	return chromedp.Run(b.ctx,
		chromedp.Evaluate(expr, nil),
	)
}

func (b *BrowserTool) GetPageSource() (string, error) {
	var source string
	err := chromedp.Run(b.ctx,
		chromedp.InnerHTML("html", &source),
	)
	return source, err
}

func (b *BrowserTool) GetURL() (string, error) {
	var url string
	err := chromedp.Run(b.ctx,
		chromedp.Location(&url),
	)
	return url, err
}

func (b *BrowserTool) GetTitle() (string, error) {
	var title string
	err := chromedp.Run(b.ctx,
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

func (o *CDPOptions) AllocatorOptions() []chromedp.ExecAllocatorOption {
	opts := chromedp.DefaultExecAllocatorOptions[:]

	if o.Headless {
		opts = append(opts, chromedp.Headless)
	}
	if o.DisableImages {
		opts = append(opts, chromedp.Flag("disable-images", true))
	}
	if o.CacheDisabled {
		opts = append(opts, chromedp.Flag("disk-cache-size", 0))
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

func RunInContext(ctx context.Context, opts *CDPOptions, fn func(*BrowserTool) error) error {
	if opts == nil {
		opts = DefaultCDPOptions()
	}

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts.AllocatorOptions()...)
	rootCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	tool, err := NewBrowserTool(rootCtx)
	if err != nil {
		return err
	}

	return fn(tool)
}
