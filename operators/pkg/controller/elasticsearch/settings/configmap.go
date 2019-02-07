// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

const (
	// SecurityPropsFile is the name of the security properties files
	SecurityPropsFile = "security.properties"
	// ManagedConfigPath is the path to our managed configuration files within the ES container
	ManagedConfigPath = "/usr/share/elasticsearch/config/managed"
)

// DefaultConfigMapData is the default config map to create for every ES pod
var DefaultConfigMapData = map[string]string{
	// With a security manager present the JVM will cache hostname lookup results indefinitely.
	// This will limit the caching to 60 seconds as we are relying on DNS for discovery in k8s.
	// See also: https://github.com/elastic/elasticsearch/pull/36570
	SecurityPropsFile: "networkaddress.cache.ttl=60\n",
}
