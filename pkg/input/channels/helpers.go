package channels

import "strings"

func streamWithMessageFallback(streamFn func(onChunk func(chunk string)) error, sendFinal func(text string) error) error {
	var accumulated strings.Builder
	err := streamFn(func(chunk string) {
		accumulated.WriteString(chunk)
	})

	final := accumulated.String()
	if err != nil {
		if strings.TrimSpace(final) != "" {
			_ = sendFinal(final + "\n\n[Error: " + err.Error() + "]")
		}
		return err
	}
	if strings.TrimSpace(final) == "" {
		return nil
	}
	return sendFinal(final)
}

func slackTSLessOrEqual(left string, right string) bool {
	if strings.TrimSpace(right) == "" {
		return false
	}
	return slackTSCompare(left, right) <= 0
}

func slackTSCompare(left string, right string) int {
	leftWhole, leftFrac := splitSlackTS(left)
	rightWhole, rightFrac := splitSlackTS(right)

	if cmp := compareNumericStrings(leftWhole, rightWhole); cmp != 0 {
		return cmp
	}

	width := len(leftFrac)
	if len(rightFrac) > width {
		width = len(rightFrac)
	}
	leftFrac += strings.Repeat("0", width-len(leftFrac))
	rightFrac += strings.Repeat("0", width-len(rightFrac))

	return compareNumericStrings(leftFrac, rightFrac)
}

func splitSlackTS(ts string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(ts), ".", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func compareNumericStrings(left string, right string) int {
	left = normalizeNumericString(left)
	right = normalizeNumericString(right)

	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func normalizeNumericString(value string) string {
	value = strings.TrimLeft(strings.TrimSpace(value), "0")
	if value == "" {
		return "0"
	}
	return value
}
