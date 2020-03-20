// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package set

import "sort"

type StringSet map[string]struct{}

func Make(strings ...string) StringSet {
	if len(strings) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(strings))
	for _, str := range strings {
		set[str] = struct{}{}
	}
	return set
}

func (set StringSet) Add(s string) {
	set[s] = struct{}{}
}

func (set StringSet) Del(s string) {
	delete(set, s)
}

func (set StringSet) Count() int {
	return len(set)
}

func (set StringSet) MergeWith(other StringSet) {
	for str := range other {
		set.Add(str)
	}
}

func (set StringSet) Has(s string) (exists bool) {
	if set != nil {
		_, exists = set[s]
	}
	return
}

func (set StringSet) AsSlice() sort.StringSlice {
	count := set.Count()
	if count == 0 {
		return nil
	}
	sl := make([]string, 0, count)
	for k := range set {
		sl = append(sl, k)
	}
	return sl
}
