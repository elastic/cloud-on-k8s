// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package configs

// APIServer hold the resolved api.* config
type APIServer struct {
	SSLEnabled       string
	KeystorePassword string
	AuthType         string
	Username         string
	Password         string
}

func (server APIServer) UseTLS() bool {
	switch server.SSLEnabled {
	case "", "true":
		return true
	case "false":
		return false
	}
	return false
}
