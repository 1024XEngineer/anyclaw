package tools

import (
	"context"
	"fmt"
	"os"
	"time"

	"anyclaw/internal/logger"
	"github.com/mafredri/cdp/protocol/emulation"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/mafredri/cdp/protocol/runtime"
	"go.uber.org/zap"
)

// BrowserCDPTool Enhanced browser automation tool with advanced CDP features
type BrowserCDPTool struct {
	*BrowserTool
}

// NewBrowserCDPTool Create enhanced CDP browser tool
func NewBrowserCDPTool(headless bool, timeout int) *BrowserCDPTool {
	return &BrowserCDPTool{
		BrowserTool: NewBrowserTool(headless, timeout),
	}
}

// BrowserPrintToPDF Generate PDF of current page
func (b *BrowserCDPTool) BrowserPrintToPDF(ctx context.Context, params map[string]interface{}) (string, error) {
	var urlStr string
	var landscape bool
	var displayHeaderFooter bool
	var printBackground bool

	if u, ok := params["url"].(string); ok {
		urlStr = u
	}
	if l, ok := params["landscape"].(bool); ok {
		landscape = l
	}
	if h, ok := params["display_header_footer"].(bool); ok {
		displayHeaderFooter = h
	}
	if p, ok := params["print_background"].(bool); ok {
		printBackground = p
	}

	logger.Debug("Browser generating PDF",
		zap.String("url", urlStr),
		zap.Bool("landscape", landscape),
		zap.Bool("print_background", printBackground),
	)

	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		if err := sessionMgr.Start(b.timeout); err != nil {
			return "", fmt.Errorf("failed to start browser session: %w", err)
		}
	}

	client, err := sessionMgr.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to get browser client: %w", err)
	}

	// Navigate if URL provided
	if urlStr != "" {
		if _, err := client.Page.Navigate(ctx, page.NewNavigateArgs(urlStr)); err != nil {
			return "", fmt.Errorf("failed to navigate: %w", err)
		}
		domContentLoaded, err := client.Page.DOMContentEventFired(ctx)
		if err != nil {
			logger.Warn("DOMContentEventFired failed", zap.Error(err))
		} else {
			defer domContentLoaded.Close()
			_, _ = domContentLoaded.Recv()
		}
		// Wait a bit more for dynamic content
		time.Sleep(2 * time.Second)
	}

	// Generate PDF
	pdfArgs := page.NewPrintToPDFArgs().
		SetLandscape(landscape).
		SetDisplayHeaderFooter(displayHeaderFooter).
		SetPrintBackground(printBackground).
		SetPreferCSSPageSize(true)

	// Set paper size (default to A4)
	pdfArgs.SetPaperWidth(8.27).SetPaperHeight(11.69) // A4 in inches

	if mt, ok := params["margin_top"].(float64); ok {
		pdfArgs.SetMarginTop(mt)
	}
	if mb, ok := params["margin_bottom"].(float64); ok {
		pdfArgs.SetMarginBottom(mb)
	}
	if ml, ok := params["margin_left"].(float64); ok {
		pdfArgs.SetMarginLeft(ml)
	}
	if mr, ok := params["margin_right"].(float64); ok {
		pdfArgs.SetMarginRight(mr)
	}

	pdfResult, err := client.Page.PrintToPDF(ctx, pdfArgs)
	if err != nil {
		return "", fmt.Errorf("failed to generate PDF: %w", err)
	}

	// Save PDF to file
	filename := fmt.Sprintf("page_%d.pdf", time.Now().Unix())
	filepath := b.outputDir + string(os.PathSeparator) + filename
	if err := os.WriteFile(filepath, pdfResult.Data, 0644); err != nil {
		return "", fmt.Errorf("failed to save PDF: %w", err)
	}

	return fmt.Sprintf("PDF saved to: %s\nSize: %d bytes", filepath, len(pdfResult.Data)), nil
}

// BrowserExtractStructuredData Extract structured data from page using schema.org, JSON-LD, or custom selectors
func (b *BrowserCDPTool) BrowserExtractStructuredData(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr, ok := params["url"].(string)
	if !ok {
		return "", fmt.Errorf("url parameter is required")
	}

	extractType := "all" // all, schema_org, json_ld, meta, custom
	if t, ok := params["type"].(string); ok {
		extractType = t
	}

	logger.Debug("Browser extracting structured data",
		zap.String("url", urlStr),
		zap.String("type", extractType),
	)

	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		if err := sessionMgr.Start(b.timeout); err != nil {
			return "", fmt.Errorf("failed to start browser session: %w", err)
		}
	}

	client, err := sessionMgr.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to get browser client: %w", err)
	}

	// Navigate to URL
	if _, err := client.Page.Navigate(ctx, page.NewNavigateArgs(urlStr)); err != nil {
		return "", fmt.Errorf("failed to navigate: %w", err)
	}
	domContentLoaded, err := client.Page.DOMContentEventFired(ctx)
	if err != nil {
		logger.Warn("DOMContentEventFired failed", zap.Error(err))
	} else {
		defer domContentLoaded.Close()
		_, _ = domContentLoaded.Recv()
	}

	// Build extraction script
	script := b.buildExtractionScript(extractType)

	// Execute extraction
	evalArgs := runtime.NewEvaluateArgs(script).SetReturnByValue(true)
	result, err := client.Runtime.Evaluate(ctx, evalArgs)
	if err != nil {
		return "", fmt.Errorf("failed to execute extraction script: %w", err)
	}

	resultJSON, err := formatCDPResult(&result.Result)
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return resultJSON, nil
}

// buildExtractionScript Build JavaScript for data extraction
func (b *BrowserCDPTool) buildExtractionScript(extractType string) string {
	baseScript := `(function() {
		const data = {};
	`

	switch extractType {
	case "schema_org":
		baseScript += `
		// Extract Schema.org microdata
		const schemaOrg = [];
		document.querySelectorAll('[itemscope]').forEach(item => {
			const schema = {};
			const itemType = item.getAttribute('itemtype');
			if (itemType) schema['@type'] = itemType;
			item.querySelectorAll('[itemprop]').forEach(prop => {
				const propName = prop.getAttribute('itemprop');
				schema[propName] = prop.textContent.trim() || prop.getAttribute('content') || prop.getAttribute('href');
			});
			if (Object.keys(schema).length > 0) schemaOrg.push(schema);
		});
		data.schema_org = schemaOrg;
		`
	case "json_ld":
		baseScript += `
		// Extract JSON-LD
		const jsonLd = [];
		document.querySelectorAll('script[type="application/ld+json"]').forEach(script => {
			try {
				jsonLd.push(JSON.parse(script.textContent));
			} catch(e) {}
		});
		data.json_ld = jsonLd;
		`
	case "meta":
		baseScript += `
		// Extract meta tags
		const meta = {};
		document.querySelectorAll('meta').forEach(tag => {
			const name = tag.getAttribute('name') || tag.getAttribute('property');
			const content = tag.getAttribute('content');
			if (name && content) meta[name] = content;
		});
		data.meta = meta;

		// Extract Open Graph
		const og = {};
		document.querySelectorAll('meta[property^="og:"]').forEach(tag => {
			const prop = tag.getAttribute('property').substring(3);
			og[prop] = tag.getAttribute('content');
		});
		data.open_graph = og;

		// Extract Twitter Card
		const twitter = {};
		document.querySelectorAll('meta[name^="twitter:"]').forEach(tag => {
			const prop = tag.getAttribute('name').substring(8);
			twitter[prop] = tag.getAttribute('content');
		});
		data.twitter_card = twitter;
		`
	case "all":
		baseScript = `(function() {
		const data = {};
		// Extract Schema.org microdata
		const schemaOrg = [];
		document.querySelectorAll('[itemscope]').forEach(item => {
			const schema = {};
			const itemType = item.getAttribute('itemtype');
			if (itemType) schema['@type'] = itemType;
			item.querySelectorAll('[itemprop]').forEach(prop => {
				const propName = prop.getAttribute('itemprop');
				schema[propName] = prop.textContent.trim() || prop.getAttribute('content') || prop.getAttribute('href');
			});
			if (Object.keys(schema).length > 0) schemaOrg.push(schema);
		});
		data.schema_org = schemaOrg;

		// Extract JSON-LD
		const jsonLd = [];
		document.querySelectorAll('script[type="application/ld+json"]').forEach(script => {
			try {
				jsonLd.push(JSON.parse(script.textContent));
			} catch(e) {}
		});
		data.json_ld = jsonLd;

		// Extract meta tags
		const meta = {};
		document.querySelectorAll('meta').forEach(tag => {
			const name = tag.getAttribute('name') || tag.getAttribute('property');
			const content = tag.getAttribute('content');
			if (name && content) meta[name] = content;
		});
		data.meta = meta;

		// Extract Open Graph
		const og = {};
		document.querySelectorAll('meta[property^="og:"]').forEach(tag => {
			const prop = tag.getAttribute('property').substring(3);
			og[prop] = tag.getAttribute('content');
		});
		data.open_graph = og;

		// Extract Twitter Card
		const twitter = {};
		document.querySelectorAll('meta[name^="twitter:"]').forEach(tag => {
			const prop = tag.getAttribute('name').substring(8);
			twitter[prop] = tag.getAttribute('content');
		});
		data.twitter_card = twitter;

		return JSON.stringify(data, null, 2);
	})();`
		return baseScript
	}

	baseScript += `
		return JSON.stringify(data, null, 2);
	})();`

	return baseScript
}

// BrowserGetMetrics Get performance metrics for the current page
func (b *BrowserCDPTool) BrowserGetMetrics(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		return "", fmt.Errorf("browser session not ready")
	}

	client, err := sessionMgr.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to get browser client: %w", err)
	}

	// Get performance metrics
	metricsResult, err := client.Performance.GetMetrics(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get performance metrics: %w", err)
	}

	// Format metrics
	script := `(function() {
		const metrics = ` + fmt.Sprintf("%v", metricsResult.Metrics) + `;
		const formatted = {};
		metrics.forEach(m => {
			formatted[m.Name] = m.Value;
		});
		return JSON.stringify(formatted, null, 2);
	})();`

	evalArgs := runtime.NewEvaluateArgs(script).SetReturnByValue(true)
	result, err := client.Runtime.Evaluate(ctx, evalArgs)
	if err != nil {
		return "", fmt.Errorf("failed to format metrics: %w", err)
	}

	resultJSON, err := formatCDPResult(&result.Result)
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return resultJSON, nil
}

// BrowserEmulateDevice Emulate a specific device (mobile, tablet, desktop)
func (b *BrowserCDPTool) BrowserEmulateDevice(ctx context.Context, params map[string]interface{}) (string, error) {
	device, ok := params["device"].(string)
	if !ok {
		return "", fmt.Errorf("device parameter is required")
	}

	logger.Debug("Browser emulating device", zap.String("device", device))

	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		return "", fmt.Errorf("browser session not ready")
	}

	client, err := sessionMgr.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to get browser client: %w", err)
	}

	// Device presets
	devices := map[string]struct {
		width  int
		height int
		ua     string
	}{
		"iphone": {
			width:  375,
			height: 667,
			ua:     "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
		},
		"ipad": {
			width:  768,
			height: 1024,
			ua:     "Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
		},
		"android": {
			width:  360,
			height: 640,
			ua:     "Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Mobile Safari/537.36",
		},
		"desktop": {
			width:  1920,
			height: 1080,
			ua:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36",
		},
	}

	preset, ok := devices[device]
	if !ok {
		return "", fmt.Errorf("unknown device: %s (available: iphone, ipad, android, desktop)", device)
	}

	// Set device metrics
	if err := client.Emulation.SetDeviceMetricsOverride(ctx, emulation.NewSetDeviceMetricsOverrideArgs(
		preset.width, preset.height, 1.0, false,
	)); err != nil {
		return "", fmt.Errorf("failed to set device metrics: %w", err)
	}

	// Set user agent
	if err := client.Emulation.SetUserAgentOverride(ctx, emulation.NewSetUserAgentOverrideArgs(preset.ua)); err != nil {
		return "", fmt.Errorf("failed to set user agent: %w", err)
	}

	return fmt.Sprintf("Successfully emulated device: %s (%dx%d)", device, preset.width, preset.height), nil
}

// BrowserSetViewport Set custom viewport size
func (b *BrowserCDPTool) BrowserSetViewport(ctx context.Context, params map[string]interface{}) (string, error) {
	width, ok := params["width"].(float64)
	if !ok {
		return "", fmt.Errorf("width parameter is required")
	}

	height, ok := params["height"].(float64)
	if !ok {
		return "", fmt.Errorf("height parameter is required")
	}

	logger.Debug("Browser setting viewport",
		zap.Float64("width", width),
		zap.Float64("height", height),
	)

	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		return "", fmt.Errorf("browser session not ready")
	}

	client, err := sessionMgr.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to get browser client: %w", err)
	}

	if err := client.Emulation.SetDeviceMetricsOverride(ctx, emulation.NewSetDeviceMetricsOverrideArgs(
		int(width), int(height), 1.0, false,
	)); err != nil {
		return "", fmt.Errorf("failed to set viewport: %w", err)
	}

	return fmt.Sprintf("Successfully set viewport to: %dx%d", int(width), int(height)), nil
}

// BrowserGetAllCookies Get all cookies for the current page
func (b *BrowserCDPTool) BrowserGetAllCookies(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		return "", fmt.Errorf("browser session not ready")
	}

	client, err := sessionMgr.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to get browser client: %w", err)
	}

	// Get document tree
	_, err = client.DOM.GetDocument(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get document: %w", err)
	}

	// Get cookies for the current URL
	script := `(function() {
		return JSON.stringify({
			cookies: document.cookie,
			url: window.location.href
		}, null, 2);
	})();`

	evalArgs := runtime.NewEvaluateArgs(script).SetReturnByValue(true)
	result, err := client.Runtime.Evaluate(ctx, evalArgs)
	if err != nil {
		return "", fmt.Errorf("failed to get cookies: %w", err)
	}

	resultJSON, err := formatCDPResult(&result.Result)
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return resultJSON, nil
}

// BrowserClose Close current tab
func (b *BrowserCDPTool) BrowserClose(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		return "", fmt.Errorf("browser session not ready")
	}

	client, err := sessionMgr.GetClient()
	if err != nil {
		return "", fmt.Errorf("failed to get browser client: %w", err)
	}

	if err := client.Page.Close(ctx); err != nil {
		return "", fmt.Errorf("failed to close page: %w", err)
	}

	return "Page closed successfully", nil
}

// BrowserCreateTab Create a new tab
func (b *BrowserCDPTool) BrowserCreateTab(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		return "", fmt.Errorf("browser session not ready")
	}

	// Create new target (tab)
	sessionMgr.mu.Lock()
	defer sessionMgr.mu.Unlock()

	pt, err := sessionMgr.devt.Create(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create new tab: %w", err)
	}

	// Get the target ID from the created target
	return fmt.Sprintf("New tab created: %v", pt), nil
}

// GetCDPTools Get all CDP-enhanced browser tools
func (b *BrowserCDPTool) GetCDPTools() []Tool {
	baseTools := b.GetTools()
	cdpTools := []Tool{
		NewBaseTool(
			"browser_print_to_pdf",
			"Generate PDF of current page or navigate to URL first",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to navigate to before generating PDF (optional)",
					},
					"landscape": map[string]interface{}{
						"type":        "boolean",
						"description": "Generate PDF in landscape orientation (default: false)",
					},
					"display_header_footer": map[string]interface{}{
						"type":        "boolean",
						"description": "Display header and footer in PDF (default: false)",
					},
					"print_background": map[string]interface{}{
						"type":        "boolean",
						"description": "Print background graphics (default: true)",
					},
					"margin_top": map[string]interface{}{
						"type":        "number",
						"description": "Top margin in inches (default: 0.4)",
					},
					"margin_bottom": map[string]interface{}{
						"type":        "number",
						"description": "Bottom margin in inches (default: 0.4)",
					},
					"margin_left": map[string]interface{}{
						"type":        "number",
						"description": "Left margin in inches (default: 0.4)",
					},
					"margin_right": map[string]interface{}{
						"type":        "number",
						"description": "Right margin in inches (default: 0.4)",
					},
				},
			},
			b.BrowserPrintToPDF,
		),
		NewBaseTool(
			"browser_extract_structured_data",
			"Extract structured data (Schema.org, JSON-LD, meta tags, Open Graph) from web page",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL of the page to extract data from",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Type of data to extract: all, schema_org, json_ld, meta (default: all)",
						"enum":        []string{"all", "schema_org", "json_ld", "meta"},
					},
				},
				"required": []string{"url"},
			},
			b.BrowserExtractStructuredData,
		),
		NewBaseTool(
			"browser_get_metrics",
			"Get performance metrics for the current page",
			map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			b.BrowserGetMetrics,
		),
		NewBaseTool(
			"browser_emulate_device",
			"Emulate a specific device (mobile, tablet, desktop)",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"device": map[string]interface{}{
						"type":        "string",
						"description": "Device to emulate",
						"enum":        []string{"iphone", "ipad", "android", "desktop"},
					},
				},
				"required": []string{"device"},
			},
			b.BrowserEmulateDevice,
		),
		NewBaseTool(
			"browser_set_viewport",
			"Set custom viewport size",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"width": map[string]interface{}{
						"type":        "number",
						"description": "Viewport width in pixels",
					},
					"height": map[string]interface{}{
						"type":        "number",
						"description": "Viewport height in pixels",
					},
				},
				"required": []string{"width", "height"},
			},
			b.BrowserSetViewport,
		),
		NewBaseTool(
			"browser_get_cookies",
			"Get all cookies for the current page",
			map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			b.BrowserGetAllCookies,
		),
		NewBaseTool(
			"browser_close",
			"Close the current browser tab",
			map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			b.BrowserClose,
		),
		NewBaseTool(
			"browser_create_tab",
			"Create a new browser tab",
			map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			b.BrowserCreateTab,
		),
	}

	return append(baseTools, cdpTools...)
}
