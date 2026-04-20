package workflow

import (
	"fmt"
	"strconv"
	"strings"
)

// EvalCondition evaluates a condition expression against a variable context.
//
// Supported operators:
//
//	Comparison: ==, !=, <, >, <=, >=
//	Logical: &&, ||, !
//	String: contains, starts_with, ends_with, empty, not_empty
//	Membership: in, not_in
//	Type checks: is_string, is_number, is_bool, is_array, is_map
//
// Variable references: $var_name, $node_id.output_key
// Literal values: strings (quoted), numbers, true, false, null
func EvalCondition(expr string, vars map[string]any) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false, fmt.Errorf("empty condition expression")
	}

	e := &evaluator{vars: vars}
	result, err := e.evalExpr(expr)
	if err != nil {
		return false, fmt.Errorf("condition evaluation error: %w", err)
	}
	return toBool(result), nil
}

type evaluator struct {
	vars map[string]any
}

func (e *evaluator) evalExpr(expr string) (any, error) {
	expr = strings.TrimSpace(expr)

	// Handle logical OR (lowest precedence)
	if idx := findLogicalOp(expr, "||"); idx >= 0 {
		left, err := e.evalExpr(expr[:idx])
		if err != nil {
			return nil, err
		}
		if toBool(left) {
			return true, nil
		}
		right, err := e.evalExpr(expr[idx+2:])
		if err != nil {
			return nil, err
		}
		return toBool(right), nil
	}

	// Handle logical AND
	if idx := findLogicalOp(expr, "&&"); idx >= 0 {
		left, err := e.evalExpr(expr[:idx])
		if err != nil {
			return nil, err
		}
		if !toBool(left) {
			return false, nil
		}
		right, err := e.evalExpr(expr[idx+2:])
		if err != nil {
			return nil, err
		}
		return toBool(right), nil
	}

	// Handle NOT
	if strings.HasPrefix(expr, "!") {
		val, err := e.evalExpr(expr[1:])
		if err != nil {
			return nil, err
		}
		return !toBool(val), nil
	}

	// Handle parenthesized expression
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		depth := 0
		for i := 0; i < len(expr); i++ {
			switch expr[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i == len(expr)-1 {
					return e.evalExpr(expr[1 : len(expr)-1])
				}
			}
		}
	}

	// Handle string functions: empty(x), not_empty(x), contains(x, y), starts_with(x, y), ends_with(x, y)
	if fnResult, ok := e.tryEvalFunc(expr); ok {
		return fnResult, nil
	}

	// Handle type checks: is_string(x), is_number(x), is_bool(x), is_array(x), is_map(x)
	if fnResult, ok := e.tryEvalTypeCheck(expr); ok {
		return fnResult, nil
	}

	// Handle "in" and "not_in" operators
	if idx := findMembershipOp(expr); idx >= 0 {
		op := expr[idx : idx+2]
		if op == "in" && idx+2 < len(expr) && expr[idx+2] != '_' {
			left, err := e.evalExpr(strings.TrimSpace(expr[:idx]))
			if err != nil {
				return nil, err
			}
			right, err := e.evalExpr(strings.TrimSpace(expr[idx+2:]))
			if err != nil {
				return nil, err
			}
			return inCollection(left, right), nil
		}
		if op == "no" && idx+6 <= len(expr) && expr[idx:idx+6] == "not_in" {
			left, err := e.evalExpr(strings.TrimSpace(expr[:idx]))
			if err != nil {
				return nil, err
			}
			right, err := e.evalExpr(strings.TrimSpace(expr[idx+6:]))
			if err != nil {
				return nil, err
			}
			return !inCollection(left, right), nil
		}
	}

	// Handle comparison operators
	if idx := findComparisonOp(expr); idx >= 0 {
		op := detectOp(expr, idx)
		left, err := e.evalExpr(strings.TrimSpace(expr[:idx]))
		if err != nil {
			return nil, err
		}
		right, err := e.evalExpr(strings.TrimSpace(expr[idx+len(op):]))
		if err != nil {
			return nil, err
		}
		return compare(left, right, op), nil
	}

	// Handle literal values and variable references
	return e.evalAtom(expr)
}

func (e *evaluator) tryEvalFunc(expr string) (any, bool) {
	funcs := map[string]func([]string) (any, error){
		"empty": func(args []string) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("empty() requires 1 argument")
			}
			val, err := e.evalExpr(args[0])
			if err != nil {
				return nil, err
			}
			return isEmpty(val), nil
		},
		"not_empty": func(args []string) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("not_empty() requires 1 argument")
			}
			val, err := e.evalExpr(args[0])
			if err != nil {
				return nil, err
			}
			return !isEmpty(val), nil
		},
		"contains": func(args []string) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("contains() requires 2 arguments")
			}
			haystack, err := e.evalExpr(args[0])
			if err != nil {
				return nil, err
			}
			needle, err := e.evalExpr(args[1])
			if err != nil {
				return nil, err
			}
			return containsValue(haystack, needle), nil
		},
		"starts_with": func(args []string) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("starts_with() requires 2 arguments")
			}
			s, err := e.evalExpr(args[0])
			if err != nil {
				return nil, err
			}
			prefix, err := e.evalExpr(args[1])
			if err != nil {
				return nil, err
			}
			return strings.HasPrefix(toStr(s), toStr(prefix)), nil
		},
		"ends_with": func(args []string) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("ends_with() requires 2 arguments")
			}
			s, err := e.evalExpr(args[0])
			if err != nil {
				return nil, err
			}
			suffix, err := e.evalExpr(args[1])
			if err != nil {
				return nil, err
			}
			return strings.HasSuffix(toStr(s), toStr(suffix)), nil
		},
		"length": func(args []string) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("length() requires 1 argument")
			}
			val, err := e.evalExpr(args[0])
			if err != nil {
				return nil, err
			}
			switch v := val.(type) {
			case string:
				return len(v), nil
			case []any:
				return len(v), nil
			case map[string]any:
				return len(v), nil
			default:
				return 0, nil
			}
		},
	}

	for name, fn := range funcs {
		prefix := name + "("
		if strings.HasPrefix(expr, prefix) && strings.HasSuffix(expr, ")") {
			inner := expr[len(prefix) : len(expr)-1]
			args := splitArgs(inner)
			result, err := fn(args)
			return result, err == nil
		}
	}
	return nil, false
}

func (e *evaluator) tryEvalTypeCheck(expr string) (any, bool) {
	typeChecks := map[string]func(any) bool{
		"is_string": func(v any) bool { _, ok := v.(string); return ok },
		"is_number": func(v any) bool {
			switch v.(type) {
			case int, int64, float64, uint:
				return true
			}
			if _, err := strconv.ParseFloat(fmt.Sprintf("%v", v), 64); err == nil {
				return true
			}
			return false
		},
		"is_bool":  func(v any) bool { _, ok := v.(bool); return ok },
		"is_array": func(v any) bool { _, ok := v.([]any); return ok },
		"is_map":   func(v any) bool { _, ok := v.(map[string]any); return ok },
		"is_null":  func(v any) bool { return v == nil },
	}

	for name, check := range typeChecks {
		prefix := name + "("
		if strings.HasPrefix(expr, prefix) && strings.HasSuffix(expr, ")") {
			inner := expr[len(prefix) : len(expr)-1]
			val, err := e.evalExpr(inner)
			if err != nil {
				return false, true
			}
			return check(val), true
		}
	}
	return nil, false
}

func (e *evaluator) evalAtom(expr string) (any, error) {
	expr = strings.TrimSpace(expr)

	// String literal
	if strings.HasPrefix(expr, `"`) && strings.HasSuffix(expr, `"`) && len(expr) >= 2 {
		return expr[1 : len(expr)-1], nil
	}
	if strings.HasPrefix(expr, `'`) && strings.HasSuffix(expr, `'`) && len(expr) >= 2 {
		return expr[1 : len(expr)-1], nil
	}

	// Boolean literals
	switch strings.ToLower(expr) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null", "nil":
		return nil, nil
	}

	// Number literal
	if num, err := strconv.ParseFloat(expr, 64); err == nil {
		return num, nil
	}

	// Variable reference
	if strings.HasPrefix(expr, "$") {
		return e.resolveVar(expr), nil
	}

	// Bare word (treat as string)
	return expr, nil
}

func (e *evaluator) resolveVar(ref string) any {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "$") {
		return ref
	}
	path := ref[1:]

	// Check node output reference: node_id.output_key
	if strings.Contains(path, ".") {
		parts := strings.SplitN(path, ".", 2)
		nodeID := parts[0]
		outputKey := parts[1]
		// Check if it's a nested path
		if strings.Contains(outputKey, ".") {
			subParts := strings.Split(outputKey, ".")
			if state, ok := e.vars["_node_outputs:"+nodeID]; ok {
				if stateMap, ok := state.(map[string]any); ok {
					return resolveNested(stateMap, subParts)
				}
			}
		}
		if state, ok := e.vars["_node_outputs:"+nodeID]; ok {
			if stateMap, ok := state.(map[string]any); ok {
				if val, ok := stateMap[outputKey]; ok {
					return val
				}
			}
		}
	}

	// Direct variable lookup
	if val, ok := e.vars[path]; ok {
		return val
	}

	// Return the reference as-is if not found
	return ref
}

func resolveNested(m map[string]any, keys []string) any {
	current := any(m)
	for _, key := range keys {
		switch v := current.(type) {
		case map[string]any:
			if val, ok := v[key]; ok {
				current = val
			} else {
				return nil
			}
		default:
			return nil
		}
	}
	return current
}

// Helpers

func findLogicalOp(expr string, op string) int {
	depth := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(expr)-len(op)+1; i++ {
		c := expr[i]
		if inString {
			if c == stringChar {
				inString = false
			}
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '"', '\'':
			inString = true
			stringChar = c
		}
		if depth == 0 && expr[i:i+len(op)] == op {
			// Make sure it's not part of a longer identifier
			if i+len(op) < len(expr) && isIdentChar(expr[i+len(op)]) {
				continue
			}
			if i > 0 && isIdentChar(expr[i-1]) {
				continue
			}
			return i
		}
	}
	return -1
}

func findComparisonOp(expr string) int {
	ops := []string{"==", "!=", "<=", ">=", "<", ">"}
	depth := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if inString {
			if c == stringChar {
				inString = false
			}
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '"', '\'':
			inString = true
			stringChar = c
		}
		if depth == 0 {
			for _, op := range ops {
				if i+len(op) <= len(expr) && expr[i:i+len(op)] == op {
					return i
				}
			}
		}
	}
	return -1
}

func findMembershipOp(expr string) int {
	depth := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if inString {
			if c == stringChar {
				inString = false
			}
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '"', '\'':
			inString = true
			stringChar = c
		}
		if depth == 0 {
			if i+6 <= len(expr) && expr[i:i+6] == "not_in" {
				if i == 0 || !isIdentChar(expr[i-1]) {
					if i+6 == len(expr) || !isIdentChar(expr[i+6]) {
						return i
					}
				}
			}
			if i+2 <= len(expr) && expr[i:i+2] == "in" {
				if (i == 0 || !isIdentChar(expr[i-1])) && (i+2 == len(expr) || !isIdentChar(expr[i+2])) {
					return i
				}
			}
		}
	}
	return -1
}

func detectOp(expr string, idx int) string {
	if idx+2 <= len(expr) {
		two := expr[idx : idx+2]
		if two == "==" || two == "!=" || two == "<=" || two == ">=" {
			return two
		}
	}
	return string(expr[idx])
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func splitArgs(s string) []string {
	var args []string
	depth := 0
	inString := false
	stringChar := byte(0)
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if c == stringChar {
				inString = false
			}
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '"', '\'':
			inString = true
			stringChar = c
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(s[start:]))
	return args
}

func compare(left, right any, op string) bool {
	lStr := toStr(left)
	rStr := toStr(right)
	lNum, lIsNum := toNum(left)
	rNum, rIsNum := toNum(right)

	if lIsNum && rIsNum {
		switch op {
		case "==":
			return lNum == rNum
		case "!=":
			return lNum != rNum
		case "<":
			return lNum < rNum
		case ">":
			return lNum > rNum
		case "<=":
			return lNum <= rNum
		case ">=":
			return lNum >= rNum
		}
	}

	switch op {
	case "==":
		return lStr == rStr
	case "!=":
		return lStr != rStr
	case "<":
		return lStr < rStr
	case ">":
		return lStr > rStr
	case "<=":
		return lStr <= rStr
	case ">=":
		return lStr >= rStr
	}
	return false
}

func inCollection(item any, collection any) bool {
	switch col := collection.(type) {
	case []any:
		for _, v := range col {
			if toStr(v) == toStr(item) {
				return true
			}
		}
	case string:
		return strings.Contains(col, toStr(item))
	case map[string]any:
		_, ok := col[toStr(item)]
		return ok
	}
	return false
}

func containsValue(haystack, needle any) bool {
	switch h := haystack.(type) {
	case string:
		return strings.Contains(h, toStr(needle))
	case []any:
		for _, v := range h {
			if toStr(v) == toStr(needle) {
				return true
			}
		}
	case map[string]any:
		_, ok := h[toStr(needle)]
		return ok
	}
	return false
}

func isEmpty(val any) bool {
	switch v := val.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	case int, int64, float64, uint:
		n, _ := toNum(val)
		return n == 0
	case bool:
		return !v
	default:
		return false
	}
}

func toBool(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "0" && v != "false" && v != "False" && v != "FALSE"
	case int, int64, float64, uint:
		n, _ := toNum(val)
		return n != 0
	case nil:
		return false
	default:
		return true
	}
}

func toStr(val any) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

func toNum(val any) (float64, bool) {
	switch v := val.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case uint:
		return float64(v), true
	case string:
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n, true
		}
		return 0, false
	default:
		return 0, false
	}
}
