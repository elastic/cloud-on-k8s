// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

// SetDefaultLabels append labels from defaults into existing. If a label already exists,
// its value is not overridden from the one in defaults.
// One use case here is to inherit user-provided labels, and append our own only if not already
// set by the user.
func SetDefaultLabels(existing map[string]string, defaults map[string]string) map[string]string {
	if existing == nil {
		existing = make(map[string]string, len(defaults))
	}
	for k, v := range defaults {
		if _, exists := existing[k]; !exists {
			existing[k] = v
		}
	}
	return existing
}
