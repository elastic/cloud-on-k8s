package common

import (
	"strings"
)

// Concat joins arguments to form a concatenated string. Uses strings.Builder
// To concatenate in the most efficient manner.
func Concat(args ...string) string {
	var result strings.Builder
	for _, arg := range args {
		result.WriteString(arg)
	}
	return result.String()
}
