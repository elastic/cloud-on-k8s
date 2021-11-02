// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package about

// Base version information.
//
// These values are meant to be overridden at runtime through go ldflags.
var (
	version       = "0.0.0"                // semantic version X.Y.Z
	buildHash     = "00000000"             // sha1 from git
	buildDate     = "1970-01-01T00:00:00Z" // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	buildSnapshot = "true"                 // boolean
)
