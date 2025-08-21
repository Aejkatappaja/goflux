package utils

import "strings"

func NormalizeEventType(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}
