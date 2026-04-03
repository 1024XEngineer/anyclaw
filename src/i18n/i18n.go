package i18n

import (
	"fmt"
	"sync"
)

type Locale struct {
	Code string
	Name string
}

var locales = map[string]map[string]string{
	"en": {
		"hello":      "Hello",
		"goodbye":    "Goodbye",
		"error":      "Error",
		"success":    "Success",
		"loading":    "Loading...",
		"no_results": "No results found",
		"confirm":    "Confirm",
		"cancel":     "Cancel",
		"save":       "Save",
		"delete":     "Delete",
		"edit":       "Edit",
		"search":     "Search",
		"settings":   "Settings",
		"help":       "Help",
		"welcome":    "Welcome to AnyClaw",
	},
	"zh": {
		"hello":      "你好",
		"goodbye":    "再见",
		"error":      "错误",
		"success":    "成功",
		"loading":    "加载中...",
		"no_results": "未找到结果",
		"confirm":    "确认",
		"cancel":     "取消",
		"save":       "保存",
		"delete":     "删除",
		"edit":       "编辑",
		"search":     "搜索",
		"settings":   "设置",
		"help":       "帮助",
		"welcome":    "欢迎使用 AnyClaw",
	},
	"zh-TW": {
		"hello":      "你好",
		"goodbye":    "再見",
		"error":      "錯誤",
		"success":    "成功",
		"loading":    "載入中...",
		"no_results": "未找到結果",
		"confirm":    "確認",
		"cancel":     "取消",
		"save":       "儲存",
		"delete":     "刪除",
		"edit":       "編輯",
		"search":     "搜尋",
		"settings":   "設定",
		"help":       "說明",
		"welcome":    "歡迎使用 AnyClaw",
	},
	"ja": {
		"hello":      "こんにちは",
		"goodbye":    "さようなら",
		"error":      "エラー",
		"success":    "成功",
		"loading":    "読み込み中...",
		"no_results": "結果が見つかりません",
		"confirm":    "確認",
		"cancel":     "キャンセル",
		"save":       "保存",
		"delete":     "削除",
		"edit":       "編集",
		"search":     "検索",
		"settings":   "設定",
		"help":       "ヘルプ",
		"welcome":    "AnyClawへようこそ",
	},
}

type Manager struct {
	mu             sync.RWMutex
	currentLocale  string
	fallbackLocale string
	custom         map[string]map[string]string
}

func New() *Manager {
	return &Manager{
		currentLocale:  "en",
		fallbackLocale: "en",
		custom:         make(map[string]map[string]string),
	}
}

func (m *Manager) SetLocale(locale string) {
	if _, ok := locales[locale]; ok {
		m.mu.Lock()
		m.currentLocale = locale
		m.mu.Unlock()
	}
}

func (m *Manager) GetLocale() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentLocale
}

func (m *Manager) ListLocales() []Locale {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Locale, 0, len(locales))
	for code, data := range locales {
		name := data["_name"]
		if name == "" {
			name = code
		}
		result = append(result, Locale{Code: code, Name: name})
	}
	return result
}

func (m *Manager) T(key string) string {
	return m.Translate(key)
}

func (m *Manager) Translate(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if custom, ok := m.custom[m.currentLocale]; ok {
		if val, ok := custom[key]; ok {
			return val
		}
	}

	if val, ok := locales[m.currentLocale][key]; ok {
		return val
	}

	if m.currentLocale != m.fallbackLocale {
		if val, ok := locales[m.fallbackLocale][key]; ok {
			return val
		}
	}

	return key
}

func (m *Manager) Translatef(key string, args ...interface{}) string {
	return fmt.Sprintf(m.Translate(key), args...)
}

func (m *Manager) AddTranslation(locale, key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.custom[locale]; !ok {
		m.custom[locale] = make(map[string]string)
	}
	m.custom[locale][key] = value
}

func (m *Manager) GetTranslations(locale string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string)

	if custom, ok := m.custom[locale]; ok {
		for k, v := range custom {
			result[k] = v
		}
	}

	if base, ok := locales[locale]; ok {
		for k, v := range base {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}

	return result
}

var defaultManager = New()

func SetLocale(locale string) {
	defaultManager.SetLocale(locale)
}

func GetLocale() string {
	return defaultManager.GetLocale()
}

func T(key string) string {
	return defaultManager.T(key)
}

func Translatef(key string, args ...interface{}) string {
	return defaultManager.Translatef(key, args...)
}
