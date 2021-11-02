// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package net

import (
	"context"
	"net"
)

// Dialer is something that can be used to create network connections.
type Dialer interface {
	// DialContext specifies the dial function for creating connections.
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}
