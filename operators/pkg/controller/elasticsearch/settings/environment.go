// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

// Environment variables applied to an Elasticsearch pod
const (
	EnvEsJavaOpts = "ES_JAVA_OPTS"

	EnvProbePasswordFile = "PROBE_PASSWORD_FILE"
	EnvProbeUsername     = "PROBE_USERNAME"

	// EnvPodName and EnvPodIP are injected as env var into the ES pod at runtime,
	// to be referenced in ES configuration file
	EnvPodName = "POD_NAME"
	EnvPodIP   = "POD_IP"
)
