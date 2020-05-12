// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

type Namer interface {
	// ConfigSecretName returns name of the Secret that hold configuration for a Beat.
	ConfigSecretName(typeName, name string) string

	// Name returns name of the Beat resource.
	Name(typeName, name string) string
}
