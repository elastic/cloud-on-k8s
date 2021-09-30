// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package net

import (
	"net"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestIPToRFCForm(t *testing.T) {
	type args struct {
		ip net.IP
	}
	tests := []struct {
		name string
		args args
		want net.IP
	}{
		{
			name: "IPv4",
			args: args{
				ip: net.IPv4(127, 0, 0, 1),
			},
			want: net.IPv4(127, 0, 0, 1).To4(),
		},
		{
			name: "IPv4 mapped IPv6",
			args: args{
				ip: net.ParseIP("::FFFF:129.144.52.38"),
			},
			want: net.IPv4(129, 144, 52, 38).To4(),
		},
		{
			name: "IPv6",
			args: args{
				ip: net.ParseIP("2001:4860:0:2001::68"),
			},
			want: net.IP{0x20, 0x01, 0x48, 0x60, 0, 0, 0x20, 0x01, 0, 0, 0, 0, 0, 0, 0x00, 0x68},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IPToRFCForm(tt.args.ip); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IPToRFCForm() = %v, want %v", len(got), len(tt.want))
			}
		})
	}
}

func TestInAddrAnyFor(t *testing.T) {
	type args struct {
		ipFamily corev1.IPFamily
	}
	tests := []struct {
		name string
		args args
		want net.IP
	}{
		{
			name: "IPv6",
			args: args{
				ipFamily: corev1.IPv6Protocol,
			},
			want: net.IPv6zero,
		},
		{
			name: "IPv4",
			args: args{
				ipFamily: corev1.IPv4Protocol,
			},
			want: net.IPv4zero,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InAddrAnyFor(tt.args.ipFamily); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("InAddrAnyFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoopbackFor(t *testing.T) {
	type args struct {
		ipFamily corev1.IPFamily
	}
	tests := []struct {
		name string
		args args
		want net.IP
	}{
		{
			name: "IPv4",
			args: args{
				ipFamily: corev1.IPv4Protocol,
			},
			want: net.ParseIP("127.0.0.1"),
		},
		{
			name: "IPv6",
			args: args{
				ipFamily: corev1.IPv6Protocol,
			},
			want: net.IPv6loopback,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LoopbackFor(tt.args.ipFamily); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoopbackFor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoopbackHostPort(t *testing.T) {
	type args struct {
		ipFamily corev1.IPFamily
		port     int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "IPv4",
			args: args{
				ipFamily: corev1.IPv4Protocol,
				port:     80,
			},
			want: "127.0.0.1:80",
		},
		{
			name: "IPv6",
			args: args{
				ipFamily: corev1.IPv6Protocol,
				port:     80,
			},
			want: "[::1]:80",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LoopbackHostPort(tt.args.ipFamily, tt.args.port); got != tt.want {
				t.Errorf("LoopbackHostPort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToIPFamily(t *testing.T) {
	type args struct {
		ipStr string
	}
	tests := []struct {
		name string
		args args
		want corev1.IPFamily
	}{
		{
			name: "Default to IPv4",
			args: args{
				ipStr: "",
			},
			want: corev1.IPv4Protocol,
		},
		{
			name: "IPv4",
			args: args{
				ipStr: "127.0.0.1",
			},
			want: corev1.IPv4Protocol,
		},
		{
			name: "IPv6 all zeros",
			args: args{
				ipStr: "::",
			},
			want: corev1.IPv6Protocol,
		},
		{
			name: "IPv4 all zeros",
			args: args{
				ipStr: "0.0.0.0",
			},
			want: corev1.IPv4Protocol,
		},
		{
			name: "IPv4-mapped IPv6",
			args: args{
				ipStr: "::FFFF:129.144.52.38",
			},
			want: corev1.IPv4Protocol,
		},
		{
			name: "IPv6",
			args: args{
				ipStr: "2001:0db8:0000:0000:0000:ff00:0042:8329",
			},
			want: corev1.IPv6Protocol,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToIPFamily(tt.args.ipStr); got != tt.want {
				t.Errorf("ToIPFamily() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIPLiteralFor(t *testing.T) {
	type args struct {
		ipOrPlaceholder string
		ipFamily        corev1.IPFamily
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "IPv4",
			args: args{
				ipOrPlaceholder: "127.0.0.1",
				ipFamily:        corev1.IPv4Protocol,
			},
			want: "127.0.0.1",
		},
		{
			name: "IPv6",
			args: args{
				ipOrPlaceholder: "::",
				ipFamily:        corev1.IPv6Protocol,
			},
			want: "[::]",
		},
		{
			name: "IPv6 Placeholder",
			args: args{
				ipOrPlaceholder: "${POD_IP}",
				ipFamily:        corev1.IPv6Protocol,
			},
			want: "[${POD_IP}]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IPLiteralFor(tt.args.ipOrPlaceholder, tt.args.ipFamily); got != tt.want {
				t.Errorf("IPLiteralFor() = %v, want %v", got, tt.want)
			}
		})
	}
}
