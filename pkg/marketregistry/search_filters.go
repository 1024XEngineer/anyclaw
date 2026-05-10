package marketregistry

import (
	"strings"
)

func appendStructuredFilterClauses(base []string, args []any, filter SearchFilter) ([]string, []any) {
	base = append(base, `NOT EXISTS (SELECT 1 FROM quarantine q WHERE q.artifact_id = a.id)`)
	if filter.Kind != "" {
		base = append(base, `a.kind = ?`)
		args = append(args, string(filter.Kind))
	}
	if filter.Source != "" {
		base = append(base, `LOWER(a.source) = LOWER(?)`)
		args = append(args, filter.Source)
	}
	if filter.Risk != "" {
		base = append(base, `LOWER(a.risk_level) = LOWER(?)`)
		args = append(args, filter.Risk)
	}
	if filter.Trust != "" {
		base = append(base, `LOWER(a.trust_level) = LOWER(?)`)
		args = append(args, filter.Trust)
	}
	if filter.Tag != "" {
		base = append(base, `EXISTS (SELECT 1 FROM json_each(a.tags_json) jt WHERE LOWER(TRIM(jt.value)) = LOWER(?))`)
		args = append(args, strings.TrimSpace(filter.Tag))
	}
	if filter.Permission != "" {
		base = append(base, `EXISTS (SELECT 1 FROM json_each(a.permissions_json) jp WHERE LOWER(TRIM(jp.value)) = LOWER(?))`)
		args = append(args, strings.TrimSpace(filter.Permission))
	}
	if filter.Publisher != "" {
		base = append(base, `LOWER(a.publisher) LIKE LOWER(?)`)
		args = append(args, "%"+strings.TrimSpace(filter.Publisher)+"%")
	}
	if filter.OS != "" {
		base = append(base, `(
			NOT EXISTS (SELECT 1 FROM json_each(a.compatibility_json, '$.os'))
			OR EXISTS (SELECT 1 FROM json_each(a.compatibility_json, '$.os') jo WHERE LOWER(TRIM(jo.value)) = LOWER(?))
		)`)
		args = append(args, strings.TrimSpace(filter.OS))
	}
	if filter.Arch != "" {
		base = append(base, `(
			NOT EXISTS (SELECT 1 FROM json_each(a.compatibility_json, '$.arch'))
			OR EXISTS (SELECT 1 FROM json_each(a.compatibility_json, '$.arch') ja WHERE LOWER(TRIM(ja.value)) = LOWER(?))
		)`)
		args = append(args, strings.TrimSpace(filter.Arch))
	}
	return base, args
}
