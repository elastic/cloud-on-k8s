// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
)

func chooseAndValidateIPFamily(ipFamilyStr string, ipFamilyDefault corev1.IPFamily) (corev1.IPFamily, error) {
	switch strings.ToLower(ipFamilyStr) {
	case "":
		return ipFamilyDefault, nil
	case "ipv4":
		return corev1.IPv4Protocol, nil
	case "ipv6":
		return corev1.IPv6Protocol, nil
	default:
		return ipFamilyDefault, fmt.Errorf("IP family can be one of: IPv4, IPv6 or \"\" to auto-detect, but was %s", ipFamilyStr)
	}
}

func validateCertExpirationFlags(validityFlag string, rotateBeforeFlag string) (time.Duration, time.Duration, error) {
	certValidity := viper.GetDuration(validityFlag)
	certRotateBefore := viper.GetDuration(rotateBeforeFlag)

	if certRotateBefore > certValidity {
		return certValidity, certRotateBefore, fmt.Errorf("%s must be larger than %s", validityFlag, rotateBeforeFlag)
	}

	return certValidity, certRotateBefore, nil
}
