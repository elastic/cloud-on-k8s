// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"

// KeystoreConfig holds keystore configuration for building ES pods.
// It supports two approaches:
//   - Init container approach (pre-9.3): Resources is set, SecretName is empty
//   - Reloadable keystore approach (9.3+): Resources is nil, SecretName is set
type KeystoreConfig struct {
	// Resources is the keystore resources for the init container approach (pre-9.3).
	// Contains the secure settings volume and init container configuration.
	// This is nil when using the reloadable keystore approach.
	Resources *keystore.Resources

	// SecretName is the name of the keystore Secret for the reloadable keystore approach (9.3+).
	// The Secret contains the pre-built keystore file that is mounted into pods.
	// This is empty when using the init container approach.
	SecretName string
}
