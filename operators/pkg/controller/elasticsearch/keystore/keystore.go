// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

const (
	managedSecretSuffix = "-keystore"
	// SecretMountPath is the mount path for keystore secrets in the init container.
	SecretMountPath = "/keystore-secrets"
	// SecretVolumeName is the name of the volume where the keystore secret is referenced.
	SecretVolumeName = "keystore"
)

// ManagedSecretName returns the name of the operator managed secret containing Elasticsearch keystore data.
func ManagedSecretName(clusterName string) string {
	return clusterName + managedSecretSuffix
}
