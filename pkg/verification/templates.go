package verification

import "time"

func RegisterDefaultTemplates(registry *TemplateRegistry) {
	registry.Register(&VerificationTemplate{
		Name:        "file-saved",
		Description: "验证文件已保存",
		Condition: &Condition{
			Type: VerificationTypeFileExists,
			Parameters: map[string]any{
				"path": "${path}",
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "file-contains-text",
		Description: "验证文件包含指定文本",
		Condition: &Condition{
			Type: VerificationTypeFileContains,
			Parameters: map[string]any{
				"path":    "${path}",
				"content": "${content}",
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "window-opened",
		Description: "验证窗口已打开",
		Condition: &Condition{
			Type: VerificationTypeWindowAppears,
			Parameters: map[string]any{
				"title": "${title}",
			},
			Timeout: 10 * time.Second,
			Retry: &RetryConfig{
				MaxAttempts:   5,
				InitialDelay:  500 * time.Millisecond,
				MaxDelay:      2 * time.Second,
				BackoffFactor: 1.5,
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "window-closed",
		Description: "验证窗口已关闭",
		Condition: &Condition{
			Type: VerificationTypeWindowNotExists,
			Parameters: map[string]any{
				"title": "${title}",
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "app-launched",
		Description: "验证应用程序已启动",
		Condition: &Condition{
			Type: VerificationTypeAppRunning,
			Parameters: map[string]any{
				"app": "${app}",
			},
			Timeout: 15 * time.Second,
			Retry: &RetryConfig{
				MaxAttempts:   10,
				InitialDelay:  1 * time.Second,
				MaxDelay:      3 * time.Second,
				BackoffFactor: 1.5,
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "app-closed",
		Description: "验证应用程序已关闭",
		Condition: &Condition{
			Type: VerificationTypeAppNotRunning,
			Parameters: map[string]any{
				"app": "${app}",
			},
			Timeout: 10 * time.Second,
			Retry: &RetryConfig{
				MaxAttempts:   5,
				InitialDelay:  500 * time.Millisecond,
				MaxDelay:      2 * time.Second,
				BackoffFactor: 1.5,
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "text-visible",
		Description: "验证指定区域包含指定文本",
		Condition: &Condition{
			Type: VerificationTypeTextContains,
			Parameters: map[string]any{
				"text":   "${text}",
				"x":      0,
				"y":      0,
				"width":  1920,
				"height": 1080,
			},
			Timeout: 5 * time.Second,
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "ocr-text-equals",
		Description: "验证 OCR 识别文本等于预期",
		Condition: &Condition{
			Type: VerificationTypeOCREquals,
			Parameters: map[string]any{
				"expected": "${expected}",
				"x":        0,
				"y":        0,
				"width":    1920,
				"height":   1080,
			},
			Timeout: 5 * time.Second,
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "ocr-text-contains",
		Description: "验证 OCR 识别文本包含预期",
		Condition: &Condition{
			Type: VerificationTypeOCRContains,
			Parameters: map[string]any{
				"expected": "${expected}",
				"x":        0,
				"y":        0,
				"width":    1920,
				"height":   1080,
			},
			Timeout: 5 * time.Second,
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "network-success",
		Description: "验证网络请求成功",
		Condition: &Condition{
			Type: VerificationTypeNetwork,
			Parameters: map[string]any{
				"url": "${url}",
			},
			Timeout: 10 * time.Second,
			Retry: &RetryConfig{
				MaxAttempts:   3,
				InitialDelay:  1 * time.Second,
				MaxDelay:      3 * time.Second,
				BackoffFactor: 2.0,
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "network-status",
		Description: "验证网络请求返回指定状态码",
		Condition: &Condition{
			Type: VerificationTypeNetworkStatus,
			Parameters: map[string]any{
				"url":    "${url}",
				"status": 200,
			},
			Timeout: 10 * time.Second,
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "clipboard-contains",
		Description: "验证剪贴板包含指定内容",
		Condition: &Condition{
			Type: VerificationTypeClipboardContains,
			Parameters: map[string]any{
				"content": "${content}",
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "element-visible",
		Description: "验证页面元素可见",
		Condition: &Condition{
			Type: VerificationTypeElementVisible,
			Parameters: map[string]any{
				"selector": "${selector}",
			},
			Timeout: 5 * time.Second,
			Retry: &RetryConfig{
				MaxAttempts:   3,
				InitialDelay:  500 * time.Millisecond,
				MaxDelay:      2 * time.Second,
				BackoffFactor: 1.5,
			},
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "element-not-visible",
		Description: "验证页面元素不可见",
		Condition: &Condition{
			Type: VerificationTypeElementNotVisible,
			Parameters: map[string]any{
				"selector": "${selector}",
			},
			Timeout: 5 * time.Second,
		},
	})

	registry.Register(&VerificationTemplate{
		Name:        "window-focused",
		Description: "验证窗口已获得焦点",
		Condition: &Condition{
			Type: VerificationTypeWindowFocused,
			Parameters: map[string]any{
				"title": "${title}",
			},
			Timeout: 3 * time.Second,
		},
	})
}

func NewVerificationExecutorWithDefaults(ctx Context) *VerificationExecutor {
	executor := NewVerificationExecutor(ctx)
	RegisterDefaultTemplates(executor.registry)
	return executor
}
