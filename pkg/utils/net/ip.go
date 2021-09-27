// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package net

import (
	"fmt"
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
	if ipFamily == corev1.IPv6Protocol {
		return net.IPv6loopback
	}
	return net.ParseIP("127.0.0.1")
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
	if len(ipStr) == 0 {
		// default to IPv4 in case no IP was given
		return corev1.IPv4Protocol
	}

	if ip := net.ParseIP(ipStr); ip.To4() != nil {
		return corev1.IPv4Protocol
	}
	return corev1.IPv6Protocol
}

// IPLiteralFor returns the given IP as a literal that can be used in a resource identifier.
// For IPv6 that means returning a bracketed version of the IP.
// The difference to net.JoinHostPort is that it also allows IP to be a placeholder that will be resolved
// to the actual IP at a later time.
func IPLiteralFor(ipOrPlaceholder string, ipFamily corev1.IPFamily) string {
	if ipFamily == corev1.IPv6Protocol {
		// IPv6: return a bracketed version of the IP
		return fmt.Sprintf("[%s]", ipOrPlaceholder)
	}
	// IPv4: leave the placeholder as is
	return ipOrPlaceholder
}
