package common

import (
	"strings"
)

// Concat joins arguments to form a concatenated string. Uses strings.Builder
// To concatenate in the most efficient manner.
func Concat(args ...string) string {
	var result strings.Builder
	for _, arg := range args {
		// it's safe to ignore the result here as strings.Builder cannot error on result.WriteString
		result.WriteString(arg) // #nosec G104
	}
	return result.String()
}

// StringInSlice returns true if the given string is found in the provided slice, else returns false
func StringInSlice(str string, list []string) bool {
	for _, s := range list {
		if s == str {
			return true
		}
	}
	return false
}

// RemoveString returns the given slice with occurences of string s removed
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
