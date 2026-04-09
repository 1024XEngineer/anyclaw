package channels

import (
	"context"
	"fmt"
	"os"
	"time"

	"anyclaw/internal/logger"
	"github.com/mdp/qrterminal"
	"go.uber.org/zap"
)

// LoginResult represents the result of a login attempt
type LoginResult struct {
	Success     bool
	ILinkBotID  string
	ILinkUserID string
	Message     string
}

// WeixinLogin handles CLI-based Weixin login
type WeixinLogin struct {
	auth      *WeixinAuth
	accountID string
}

// NewWeixinLogin creates a new login handler
func NewWeixinLogin(accountID string) (*WeixinLogin, error) {
	auth, err := NewWeixinAuth(NewWeixinAPIClient("", ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create auth handler: %w", err)
	}

	return &WeixinLogin{
		auth:      auth,
		accountID: accountID,
	}, nil
}

// Login performs the login flow
func (l *WeixinLogin) Login(ctx context.Context) (*LoginResult, error) {
	return l.LoginWithDisplay(ctx, l.displayQRCode)
}

// LoginWithDisplay performs the login flow with a custom QR code display function
func (l *WeixinLogin) LoginWithDisplay(ctx context.Context, displayQR func(url string) error) (*LoginResult, error) {
	// Start QR code login (GET request)
	qrResp, err := l.auth.StartQRLogin(ctx)
	if err != nil {
		return &LoginResult{
			Success: false,
			Message: fmt.Sprintf("Failed to get QR code: %v", err),
		}, err
	}

	// Display QR code - qrcode_img_content is the URL to encode into QR
	if displayQR != nil {
		if err := displayQR(qrResp.QRCodeImgContent); err != nil {
			return &LoginResult{
				Success: false,
				Message: fmt.Sprintf("Failed to display QR code: %v", err),
			}, err
		}
	}

	// Wait for login with status updates (GET polling)
	statusChan := make(chan string, 1)
	resultChan := make(chan *LoginResult, 1)

	go func() {
		tokenInfo, err := l.auth.WaitForLogin(ctx, qrResp.QRCode, func(status string) {
			select {
			case statusChan <- status:
			default:
			}
		})

		if err != nil {
			resultChan <- &LoginResult{
				Success: false,
				Message: fmt.Sprintf("Login failed: %v", err),
			}
			return
		}

		// Save token
		if err := l.auth.SaveToken(l.accountID, tokenInfo); err != nil {
			logger.Warn("Failed to save token", zap.Error(err))
		}

		resultChan <- &LoginResult{
			Success:     true,
			ILinkBotID:  tokenInfo.ILinkBotID,
			ILinkUserID: tokenInfo.ILinkUserID,
			Message:     "Login successful",
		}
	}()

	// Print status updates
	for {
		select {
		case <-ctx.Done():
			return &LoginResult{
				Success: false,
				Message: "Login cancelled",
			}, ctx.Err()
		case status := <-statusChan:
			switch status {
			case "scaned":
				fmt.Println("\n👀 已扫码，在微信继续操作...")
			case "expired":
				return &LoginResult{
					Success: false,
					Message: "QR code expired, please try again",
				}, nil
			}
		case result := <-resultChan:
			return result, nil
		}
	}
}

// displayQRCode displays the QR code in terminal
func (l *WeixinLogin) displayQRCode(qrURL string) error {
	fmt.Println("\n📱 Weixin Login")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
	fmt.Println("使用微信扫描以下二维码完成登录：")
	fmt.Println()
	fmt.Println("  1. 打开手机微信")
	fmt.Println("  2. 进入「我」>「设置」>「设备」")
	fmt.Println("  3. 点击「扫一扫」扫描二维码")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()

	// Generate and display QR code in terminal
	// qrURL is the URL to encode into the QR code
	qrterminal.Generate(qrURL, qrterminal.L, os.Stdout)

	fmt.Println()
	fmt.Println("等待扫码...")

	return nil
}

// Logout logs out from Weixin
func (l *WeixinLogin) Logout() error {
	return l.auth.DeleteToken(l.accountID)
}

// Status checks the login status
func (l *WeixinLogin) Status() (*TokenInfo, error) {
	return l.auth.LoadToken(l.accountID)
}

// RunWeixinLogin is the main entry point for CLI login
func RunWeixinLogin(accountID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	login, err := NewWeixinLogin(accountID)
	if err != nil {
		return fmt.Errorf("failed to create login handler: %w", err)
	}

	result, err := login.Login(ctx)
	if err != nil {
		return err
	}

	if result.Success {
		fmt.Printf("\n✅ 登录成功！\n")
		fmt.Printf("   Bot ID:  %s\n", result.ILinkBotID)
		fmt.Printf("   User ID: %s\n", result.ILinkUserID)
		fmt.Printf("\n现在可以启动 anyclaw 服务使用微信通道了。\n")
	} else {
		fmt.Printf("\n❌ 登录失败: %s\n", result.Message)
	}

	return nil
}

// RunWeixinLogout logs out from Weixin
func RunWeixinLogout(accountID string) error {
	login, err := NewWeixinLogin(accountID)
	if err != nil {
		return fmt.Errorf("failed to create login handler: %w", err)
	}

	if err := login.Logout(); err != nil {
		return fmt.Errorf("failed to logout: %w", err)
	}

	fmt.Printf("✅ 已登出账号: %s\n", accountID)
	return nil
}

// RunWeixinStatus checks the login status
func RunWeixinStatus(accountID string) error {
	login, err := NewWeixinLogin(accountID)
	if err != nil {
		return fmt.Errorf("failed to create login handler: %w", err)
	}

	tokenInfo, err := login.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if tokenInfo == nil {
		fmt.Printf("❌ 账号 %s 尚未登录\n", accountID)
		fmt.Println("运行 'anyclaw channels weixin login' 进行登录。")
		return nil
	}

	fmt.Printf("账号: %s\n", accountID)
	fmt.Printf("  Bot ID:  %s\n", tokenInfo.ILinkBotID)
	fmt.Printf("  User ID: %s\n", tokenInfo.ILinkUserID)

	if tokenInfo.ExpiresAt > 0 {
		expiresAt := time.Unix(tokenInfo.ExpiresAt, 0)
		if time.Now().After(expiresAt) {
			fmt.Printf("  状态: ❌ Token 已过期 (%s)\n", expiresAt.Format(time.RFC3339))
		} else {
			remaining := time.Until(expiresAt).Round(time.Minute)
			fmt.Printf("  状态: ✅ 已登录 (剩余 %s)\n", remaining)
		}
	} else {
		fmt.Printf("  状态: ✅ 已登录\n")
	}

	return nil
}
