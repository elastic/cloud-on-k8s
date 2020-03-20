// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package maps

// IsSubset compares two maps to determine if one of them is fully contained in the other.
func IsSubset(toCheck, fullSet map[string]string) bool {
	if len(toCheck) > len(fullSet) {
		return false
	}

	for k, v := range toCheck {
		if currValue, ok := fullSet[k]; !ok || currValue != v {
			return false
		}
	}

	return true
}

// Merge merges source into destination, overwriting existing values if necessary.
func Merge(dest, src map[string]string) map[string]string {
	if dest == nil {
		if src == nil {
			return nil
		}
		dest = make(map[string]string, len(src))
	}

	for k, v := range src {
		dest[k] = v
	}

	return dest
}

// MergePreservingExistingKeys merges source into destination while skipping any keys that exist in the destination.
func MergePreservingExistingKeys(dest, src map[string]string) map[string]string {
	if dest == nil {
		if src == nil {
			return nil
		}
		dest = make(map[string]string, len(src))
	}

	for k, v := range src {
		if _, exists := dest[k]; !exists {
			dest[k] = v
		}
	}

	return dest
}

// ContainsKeys determines if a set of label (keys) are present in a map of labels (keys and values).
func ContainsKeys(m map[string]string, labels ...string) bool {
	for _, label := range labels {
		if _, exists := m[label]; !exists {
			return false
		}
	}
	return true
}
