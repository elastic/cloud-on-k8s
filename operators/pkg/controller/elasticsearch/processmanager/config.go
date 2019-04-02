// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

const (
	// DefaultPort used by the process-manager HTTP api
	DefaultPort = 8080
)

// Config contains configuration parameters for the process manager.
type Config struct {
	// Process name to manage (used only for display)
	ProcessName string
	// Process command to manage
	ProcessCmd string
	// Boolean to enable/disable the child processes reaper
	EnableReaper bool

	// Port of the HTTP server
	HTTPPort int
	// Boolean to enable/disable TLS for the HTTP server
	EnableTLS bool
	// Path to the certificate file used to secure the HTTP server
	CertPath string
	// Path to the private key file used to secure the HTTP server
	KeyPath string

	// Boolean to enable/disable the keystore updater
	EnableKeystoreUpdater bool

	// Boolean to enable/disable the basic memory metrics endpoint (/debug/vars)
	EnableExpVars bool
	// Boolean to enable/disable the runtime profiling data endpoint (/debug/pprof/)
	EnableProfiler bool
}
