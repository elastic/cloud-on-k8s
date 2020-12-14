// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

type Replacement struct {
	Path        []string
	Expected    interface{}
	Replacement interface{}
}

func (r Replacement) apply(out untypedDict) {
	replaceTarget := out
	for i, p := range r.Path {
		// last element?
		if i == len(r.Path)-1 {
			// matches expected value?
			actual, exists := replaceTarget[p]
			if exists && actual == r.Expected {
				// do the replacement
				replaceTarget[p] = r.Replacement
				return
			}
		}

		v, exists := replaceTarget[p].(untypedDict)
		if !exists {
			return
		}
		replaceTarget = v
	}
}
