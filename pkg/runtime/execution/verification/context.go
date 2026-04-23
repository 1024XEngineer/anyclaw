package verification

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type DefaultContext struct {
	desktopTools DesktopTools
}

type DesktopTools interface {
	WindowAppears(title string) (bool, error)
	WindowFocused(title string) (bool, error)
	TextContains(x, y, width, height int, text string) (bool, error)
	OCRText(x, y, width, height int) (string, error)
	Clipboard() (string, error)
	ElementVisible(selector string) (bool, error)
	Screenshot() ([]byte, error)
}

func NewDefaultContext() *DefaultContext {
	return &DefaultContext{}
}

func (c *DefaultContext) FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (c *DefaultContext) FileContains(path string, content string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), content), nil
}

func (c *DefaultContext) FileMatches(path string, pattern string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return regexp.MatchString(pattern, string(data))
}

func (c *DefaultContext) WindowAppears(title string) (bool, error) {
	if c.desktopTools != nil {
		return c.desktopTools.WindowAppears(title)
	}
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`$title = %s; Get-Process | Where-Object {$_.MainWindowTitle -like ('*' + $title + '*')}`, quotePowerShellLiteral(title)))
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

func (c *DefaultContext) WindowFocused(title string) (bool, error) {
	if c.desktopTools != nil {
		return c.desktopTools.WindowFocused(title)
	}
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`$title = %s; (Get-Process | Where-Object {$_.MainWindowTitle -like ('*' + $title + '*')} | Select-Object -First 1).MainWindowTitle`, quotePowerShellLiteral(title)))
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(output)) != "", nil
}

func (c *DefaultContext) TextContains(x, y, width, height int, text string) (bool, error) {
	if c.desktopTools != nil {
		return c.desktopTools.TextContains(x, y, width, height, text)
	}
	actual, err := c.OCRText(x, y, width, height)
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(actual), strings.ToLower(text)), nil
}

func (c *DefaultContext) OCRText(x, y, width, height int) (string, error) {
	if c.desktopTools != nil {
		return c.desktopTools.OCRText(x, y, width, height)
	}
	return "", fmt.Errorf("OCR not implemented: provide DesktopTools")
}

func (c *DefaultContext) Clipboard() (string, error) {
	if c.desktopTools != nil {
		return c.desktopTools.Clipboard()
	}
	cmd := exec.Command("powershell", "-Command", "Get-Clipboard")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *DefaultContext) ClipboardContains(content string) (bool, bool) {
	clipboard, err := c.Clipboard()
	if err != nil {
		return false, false
	}
	if clipboard == "" {
		return false, false
	}
	return strings.Contains(clipboard, content), true
}

func (c *DefaultContext) NetworkRequest(url string) (int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func (c *DefaultContext) AppRunning(appName string) (bool, error) {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`$name = %s; Get-Process -Name $name -ErrorAction SilentlyContinue`, quotePowerShellLiteral(appName)))
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

func (c *DefaultContext) AppState(appName string) (map[string]any, error) {
	running, err := c.AppRunning(appName)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"running": running,
		"name":    appName,
	}, nil
}

func (c *DefaultContext) ElementVisible(selector string) (bool, error) {
	if c.desktopTools != nil {
		return c.desktopTools.ElementVisible(selector)
	}
	return false, fmt.Errorf("element visible check not implemented")
}

func (c *DefaultContext) Screenshot() ([]byte, error) {
	if c.desktopTools != nil {
		return c.desktopTools.Screenshot()
	}
	return nil, fmt.Errorf("screenshot not implemented")
}

func (c *DefaultContext) CustomVerify(name string, params map[string]any) (*VerificationResult, error) {
	return nil, fmt.Errorf("custom verify '%s' not implemented", name)
}

func (c *DefaultContext) SetDesktopTools(tools DesktopTools) {
	c.desktopTools = tools
}

func quotePowerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
