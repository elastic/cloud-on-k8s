// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestCAVolumeName(t *testing.T) {
	name := CAVolumeName(
		"es-monitoring",
		"extremely-long-and-unwieldy-namespace-that-exceeds-the-limit",
		"extremely-long-and-unwieldy-name-that-exceeds-the-limit",
	)
	require.LessOrEqual(t, len(name), MaxVolumeNameLength)
	require.Empty(t, validation.IsDNS1123Label(name), name)
	require.Equal(t, "es-monitoring-954c60-ca", name)
	require.Equal(t, name, CAVolumeName(
		"es-monitoring",
		"extremely-long-and-unwieldy-namespace-that-exceeds-the-limit",
		"extremely-long-and-unwieldy-name-that-exceeds-the-limit",
	), "volume names must be deterministic")
}

func TestClientCertVolumeName(t *testing.T) {
	ns := "extremely-long-and-unwieldy-namespace-that-exceeds-the-limit"
	n := "extremely-long-and-unwieldy-name-that-exceeds-the-limit"
	name := ClientCertVolumeName("es-monitoring", ns, n)
	require.LessOrEqual(t, len(name), MaxVolumeNameLength)
	require.Empty(t, validation.IsDNS1123Label(name), name)
	require.Equal(t, "es-monitoring-954c60-client-cert", name)
	require.NotEqual(t, fmt.Sprintf("%s-client-cert", CAVolumeName("es-monitoring", ns, n)), name)
}

func TestCertVolumeNamesCustomerReproduction(t *testing.T) {
	ns := "apl-ops-intelligence-monitoring"
	n := "elasticsearch-aws-nonprod-monitoring"
	otherNS := "apl-ops-intelligence-monitorinx"

	caName := CAVolumeName("es", ns, n)
	require.LessOrEqual(t, len(caName), MaxVolumeNameLength)
	require.Empty(t, validation.IsDNS1123Label(caName), caName)
	require.Regexp(t, `^es-[0-9a-f]{6}-ca$`, caName)

	clientName := ClientCertVolumeName("es", ns, n)
	require.LessOrEqual(t, len(clientName), MaxVolumeNameLength)
	require.Empty(t, validation.IsDNS1123Label(clientName), clientName)
	require.Regexp(t, `^es-[0-9a-f]{6}-client-cert$`, clientName)

	require.NotEqual(t, caName, CAVolumeName("es", otherNS, n))
	require.NotEqual(t, clientName, ClientCertVolumeName("es", otherNS, n))
}
