// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

// chart defines the elements of a Helm chart.
type chart struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Dependencies []dependency `json:"dependencies"`
	srcPath      string
}

// dependency is a dependency of a Helm chart.
type dependency struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository"`
}

// charts is a slice of Helm charts.
type charts []chart

// chartNames returns a slice of the names of Helm charts.
func (cs charts) chartNames() []string {
	names := make([]string, len(cs))
	for i, chart := range cs {
		names[i] = chart.Name
	}
	return names
}
