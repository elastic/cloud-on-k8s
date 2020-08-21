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

// IpToRFCForm normalizes the IP address given to fit the expected network byte order octet form described in
// https://tools.ietf.org/html/rfc5280#section-4.2.1.6
func IpToRFCForm(ip net.IP) net.IP {
	if ip := ip.To4(); ip != nil {
		return ip
	}
	return ip.To16()
}

func LoopbackFor(ip net.IP) net.IP {
	lb := net.IPv6loopback
	if ip.To4() != nil {
		lb = net.ParseIP("127.0.0.1")
	}
	return lb
}
