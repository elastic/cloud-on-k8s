// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package net

import (
	"net"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// IPToRFCForm normalizes the IP address given to fit the expected network byte order octet form described in
// https://tools.ietf.org/html/rfc5280#section-4.2.1.6
func IPToRFCForm(ip net.IP) net.IP {
	if ip := ip.To4(); ip != nil {
		return ip
	}
	return ip.To16()
}

// LoopbackFor returns the loopback address for the given IP family.
func LoopbackFor(ipFamily corev1.IPFamily) net.IP {
	if ipFamily == corev1.IPv4Protocol {
		return net.ParseIP("127.0.0.1")
	}
	return net.IPv6loopback
}

// LoopbackHostPort formats a loopback address and port correctly for the given IP family.
func LoopbackHostPort(ipFamily corev1.IPFamily, port int) string {
	return net.JoinHostPort(LoopbackFor(ipFamily).String(), strconv.Itoa(port))
}

// InAddrAnyFor returns the special IP address to bind to any IP address (0.0.0.0 or ::) depending on IP family.
func InAddrAnyFor(ipFamily corev1.IPFamily) net.IP {
	if ipFamily == corev1.IPv4Protocol {
		return net.IPv4zero
	}
	return net.IPv6zero
}

// ToIPFamily tries to detect the IP family (IPv4 or IPv6) based on the given IP string.
func ToIPFamily(ipStr string) corev1.IPFamily {
	if ip := net.ParseIP(ipStr); len(ip) == net.IPv6len {
		return corev1.IPv6Protocol
	}
	return corev1.IPv4Protocol
}
