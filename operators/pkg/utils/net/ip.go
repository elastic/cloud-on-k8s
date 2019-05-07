// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package net

import (
	"net"
)

// MaybeIPTo4 attempts to convert the provided net.IP to a 4-byte representation if possible, otherwise does nothing.
func MaybeIPTo4(ipAddress net.IP) net.IP {
	if ip := ipAddress.To4(); ip != nil {
		return ip
	}
	return ipAddress
}
