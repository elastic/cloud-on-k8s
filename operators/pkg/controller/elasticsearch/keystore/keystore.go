// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

const (
	ManagedSecretName = "keystore-secret"
	// SecretMountPath Mountpath for keystore secrets in init container.
	SecretMountPath = "/keystore-secrets"
	// SecretVolumeName is the name of the volume where the keystore secret is referenced.
	SecretVolumeName = "keystore"
)
