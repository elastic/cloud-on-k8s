// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

// keystoreInitContainerParameters returns the keystore init container parameters for the given APM Server version.
func keystoreInitContainerParameters(v version.Version) keystore.InitContainerParameters {
	params := initContainerParameters
	if SupportsKeystoreAddFromDir(v) {
		// Invoke the binary directly as images without a shell (e.g. wolfi) cannot run the keystore script.
		// --force creates the keystore if missing and overwrites existing keys without prompting, which also
		// makes the command safe to re-run if the init container restarts.
		params.Command = []string{
			ApmServerBin, "keystore", "add", "--from-dir", params.SecureSettingsVolumeMountPath, "--force",
		}
	}
	return params
}

// SupportsKeystoreAddFromDir returns true if the `keystore add` command of the given APM Server version supports
// the --from-dir flag, see https://github.com/elastic/apm-server/pull/22305.
func SupportsKeystoreAddFromDir(v version.Version) bool {
	switch {
	case v.Major == 8 && v.Minor == 19:
		return v.GTE(version.MinFor(8, 19, 26))
	case v.Major == 9 && v.Minor == 5:
		return v.GTE(version.MinFor(9, 5, 8))
	default:
		return v.GTE(version.MinFor(9, 6, 1))
	}
}
