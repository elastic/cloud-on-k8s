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
