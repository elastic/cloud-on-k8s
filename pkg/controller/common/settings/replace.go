// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

// Replacement is a utility to replace an element in an untypedDict at Path with Replacement iff the element is equal
// to Expected. This is mainly intended to enable a workaround around for https://github.com/elastic/cloud-on-k8s/issues/3718
type Replacement struct {
	Path        []string
	Expected    interface{}
	Replacement interface{}
}

func (r Replacement) apply(out untypedDict) {
	dict := out
	for idx, pathSegment := range r.Path {
		// last path element?
		if idx == len(r.Path)-1 {
			// matches expected value?
			actual, exists := dict[pathSegment]
			if exists && actual == r.Expected {
				// do the replacement
				dict[pathSegment] = r.Replacement
				return
			}
		}
		// move one level down into the structure of nested maps
		nestedDict, exists := dict[pathSegment].(untypedDict)
		if !exists {
			return
		}
		dict = nestedDict
	}
}
