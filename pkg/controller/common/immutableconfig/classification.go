// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package immutableconfig

import (
	"fmt"
	"sort"
)

// Classification categorizes how a config file is handled during reconciliation.
type Classification int

const (
	Unclassified Classification = iota // Unknown classification, will cause an error
	Immutable                          // Content-addressed, never updated in-place
	Dynamic                            // Hot-reloadable, updated in-place
)

// Classifier maps file names to their classification.
type Classifier interface {
	Classify(filename string) Classification
}

// MapClassifier is a simple map-based implementation of Classifier.
type MapClassifier map[string]Classification

// Classify returns the classification for the given filename.
func (m MapClassifier) Classify(filename string) Classification {
	return m[filename]
}

// NamesWithClassification returns all names in the classifier that have the given classification.
// The returned slice is sorted for deterministic behavior.
func (m MapClassifier) NamesWithClassification(c Classification) []string {
	var names []string
	for name, classification := range m {
		if classification == c {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// SplitByClassification splits data into immutable and dynamic maps based on the classifier.
// Returns an error if any file is unclassified.
func SplitByClassification(data map[string][]byte, classifier Classifier) (immutable, dynamic map[string][]byte, err error) {
	immutable = make(map[string][]byte)
	dynamic = make(map[string][]byte)

	for name, content := range data {
		switch classifier.Classify(name) {
		case Immutable:
			immutable[name] = content
		case Dynamic:
			dynamic[name] = content
		case Unclassified:
			return nil, nil, fmt.Errorf("unclassified config file: %s", name)
		}
	}
	return immutable, dynamic, nil
}

// SplitStringByClassification splits string data into immutable and dynamic maps.
// This is a convenience wrapper for ConfigMap data which uses map[string]string.
func SplitStringByClassification(data map[string]string, classifier Classifier) (immutable, dynamic map[string]string, err error) {
	immutable = make(map[string]string)
	dynamic = make(map[string]string)

	for name, content := range data {
		switch classifier.Classify(name) {
		case Immutable:
			immutable[name] = content
		case Dynamic:
			dynamic[name] = content
		case Unclassified:
			return nil, nil, fmt.Errorf("unclassified config file: %s", name)
		}
	}
	return immutable, dynamic, nil
}
