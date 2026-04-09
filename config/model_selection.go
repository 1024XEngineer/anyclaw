package config

import (
	"encoding"
	"fmt"
)

// ModelSelection describes the primary model ref and optional fallbacks.
type ModelSelection struct {
	Primary   string   `mapstructure:"primary" json:"primary"`
	Fallbacks []string `mapstructure:"fallbacks" json:"fallbacks"`
}

// Effective returns the first configured model reference (primary first, then fallbacks).
func (m *ModelSelection) Effective() string {
	if m == nil {
		return ""
	}

	if m.Primary != "" {
		return m.Primary
	}

	for _, fallback := range m.Fallbacks {
		if fallback != "" {
			return fallback
		}
	}

	return ""
}

// String implements fmt.Stringer so ModelSelection prints as the effective model ref.
func (m ModelSelection) String() string {
	return m.Effective()
}

// UnmarshalText allows mapstructure to decode a bare string into the primary ref.
func (m *ModelSelection) UnmarshalText(text []byte) error {
	if m == nil {
		return fmt.Errorf("cannot unmarshal into nil ModelSelection")
	}

	m.Primary = string(text)
	return nil
}

// MarshalText ensures the textual representation matches the primary ref.
func (m ModelSelection) MarshalText() ([]byte, error) {
	return []byte(m.Effective()), nil
}

var _ encoding.TextMarshaler = ModelSelection{}
var _ encoding.TextUnmarshaler = (*ModelSelection)(nil)
