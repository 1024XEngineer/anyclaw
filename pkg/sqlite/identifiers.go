package sqlite

import (
	"fmt"
	"strings"
	"unicode"
)

func sanitizeIdentifier(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("sqlite: identifier is required")
	}

	parts := strings.Split(name, ".")
	for _, part := range parts {
		if err := validateIdentifierPart(part); err != nil {
			return "", err
		}
	}

	return strings.Join(parts, "."), nil
}

func sanitizeColumn(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "*":
		return name, nil
	case strings.HasSuffix(name, ".*"):
		base, err := sanitizeIdentifier(strings.TrimSuffix(name, ".*"))
		if err != nil {
			return "", err
		}
		return base + ".*", nil
	default:
		return sanitizeIdentifier(name)
	}
}

func sanitizeIdentifierList(items []string, sanitize func(string) (string, error)) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}

	sanitized := make([]string, 0, len(items))
	for _, item := range items {
		clean, err := sanitize(item)
		if err != nil {
			return nil, err
		}
		sanitized = append(sanitized, clean)
	}

	return sanitized, nil
}

func sanitizeOrderByClause(clause string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(clause))
	if len(fields) == 0 {
		return "", fmt.Errorf("sqlite: order by clause is required")
	}
	if len(fields) > 2 {
		return "", fmt.Errorf("sqlite: invalid order by clause %q", clause)
	}

	column, err := sanitizeIdentifier(fields[0])
	if err != nil {
		return "", err
	}
	if len(fields) == 1 {
		return column, nil
	}

	direction := strings.ToUpper(fields[1])
	if direction != "ASC" && direction != "DESC" {
		return "", fmt.Errorf("sqlite: invalid order by direction %q", fields[1])
	}

	return column + " " + direction, nil
}

func validateIdentifierPart(part string) error {
	part = strings.TrimSpace(part)
	if part == "" {
		return fmt.Errorf("sqlite: identifier is required")
	}

	for i, r := range part {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return fmt.Errorf("sqlite: invalid identifier %q", part)
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("sqlite: invalid identifier %q", part)
		}
	}

	return nil
}
