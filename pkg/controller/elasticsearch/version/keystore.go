// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package version

import "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"

// KeystorePasswordMinVersion is the minimum Elasticsearch version that
// supports Secret-mounted KEYSTORE_PASSWORD_FILE with group-readable modes.
var KeystorePasswordMinVersion = version.MinFor(9, 4, 0)
