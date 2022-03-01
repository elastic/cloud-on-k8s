// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stringsutil

import (
	"sort"
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

// StringsInSlice returns true if the given strings are found in the provided slice, else returns false
func StringsInSlice(strings []string, slice []string) bool {
	asMap := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		asMap[s] = struct{}{}
	}
	for _, s := range strings {
		if _, exists := asMap[s]; !exists {
			return false
		}
	}
	return true
}

// Difference returns the elements in `a` that aren't in `b` and the elements in `b` that aren't in `a`
func Difference(a, b []string) ([]string, []string) {
	// Sort strings to have a stable output
	sort.Strings(a)
	sort.Strings(b)
	mb := SliceToMap(b)
	ma := make(map[string]struct{}, len(a))
	var inA []string
	for _, x := range a {
		ma[x] = struct{}{}
		if _, found := mb[x]; !found {
			inA = append(inA, x)
		}
	}
	var inB []string
	for _, x := range b {
		if _, found := ma[x]; !found {
			inB = append(inB, x)
		}
	}
	return inA, inB
}

// RemoveStringInSlice returns a new slice with all occurrences of s removed,
// keeping the given slice unmodified
func RemoveStringInSlice(s string, slice []string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return result
}

func SliceToMap(slice []string) map[string]struct{} {
	m := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		m[s] = struct{}{}
	}
	return m
}

func SortStringSlice(s []string) {
	sort.SliceStable(s, func(i, j int) bool {
		return s[i] < s[j]
	})
}
