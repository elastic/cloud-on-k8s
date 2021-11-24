// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package about

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8suuid "k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
)

const (
	// UUIDCfgMapName is the name for the config map  containing the operator UUID.
	// The operator UUID is then used in the OperatorInfo.
	UUIDCfgMapName = "elastic-operator-uuid"
	// UUIDCfgMapName is used for the operator UUID inside a config map.
	UUIDCfgMapKey = "uuid"
)

var (
	// lookup of valid distribution channels
	knownDistributionChannels = map[string]struct{}{
		"all-in-one":                   {},
		"helm":                         {},
		"upstream-community-operators": {},
		"community-operators":          {},
		"certified-operators":          {},
		"ironbank":                     {},
		"image":                        {},
	}
)

var defaultOperatorNamespaces = []string{"elastic-system"}

// OperatorInfo contains information about the operator.
type OperatorInfo struct {
	OperatorUUID            types.UID `json:"operator_uuid"`
	CustomOperatorNamespace bool      `json:"custom_operator_namespace"`
	Distribution            string    `json:"distribution"`
	DistributionChannel     string    `json:"distributionChannel"`
	BuildInfo               BuildInfo `json:"build"`
}

// BuildInfo contains build metadata information.
type BuildInfo struct {
	Version  string `json:"version"`
	Hash     string `json:"hash"`
	Date     string `json:"date"`
	Snapshot string `json:"snapshot"`
}

// VersionString returns the version information formatted according to the SemVer specification.
func (bi BuildInfo) VersionString() string {
	var sb strings.Builder

	sb.WriteString(bi.Version)

	if bi.Snapshot == "true" && !strings.HasSuffix(bi.Version, "-SNAPSHOT") {
		sb.WriteString("-SNAPSHOT")
	}

	sb.WriteString("+")
	sb.WriteString(bi.Hash)

	return sb.String()
}

// IsDefined returns true if the info's default values have been replaced.
func (i OperatorInfo) IsDefined() bool {
	return i.OperatorUUID != "" &&
		i.Distribution != "" &&
		i.BuildInfo.Version != "0.0.0" &&
		i.BuildInfo.Hash != "00000000" &&
		i.BuildInfo.Date != "1970-01-01T00:00:00Z"
}

// GetOperatorInfo returns an OperatorInfo given an operator client, a Kubernetes client config, an operator namespace.
func GetOperatorInfo(clientset kubernetes.Interface, operatorNs, distributionChannel string) (OperatorInfo, error) {
	operatorUUID, err := getOperatorUUID(context.Background(), clientset, operatorNs)
	if err != nil {
		return OperatorInfo{}, err
	}

	distribution, err := getDistribution(clientset)
	if err != nil {
		return OperatorInfo{}, err
	}

	customOperatorNs := true
	for _, ns := range defaultOperatorNamespaces {
		if operatorNs == ns {
			customOperatorNs = false
		}
	}

	// Check if reported channel is known to us. Passing bogus value to the
	// operator is treated the same as not passing any value at all
	if _, ok := knownDistributionChannels[distributionChannel]; !ok {
		distributionChannel = ""
	}

	return OperatorInfo{
		OperatorUUID:            operatorUUID,
		CustomOperatorNamespace: customOperatorNs,
		Distribution:            distribution,
		DistributionChannel:     distributionChannel,
		BuildInfo:               GetBuildInfo(),
	}, nil
}

// getOperatorUUID returns the operator UUID by retrieving a config map or creating it if it does not exist.
func getOperatorUUID(ctx context.Context, clientset kubernetes.Interface, operatorNs string) (types.UID, error) {
	c := clientset.CoreV1().ConfigMaps(operatorNs)

	// get the config map
	reconciledCfgMap, err := c.Get(ctx, UUIDCfgMapName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return types.UID(""), err
	}

	// or create it
	if err != nil && apierrors.IsNotFound(err) {
		newUUID := k8suuid.NewUUID()
		cfgMap := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNs,
				Name:      UUIDCfgMapName,
			},
			Data: map[string]string{
				UUIDCfgMapKey: string(newUUID),
			},
		}
		_, err = c.Create(ctx, &cfgMap, metav1.CreateOptions{})
		if err != nil {
			return types.UID(""), err
		}

		return newUUID, nil
	}

	UUID, ok := reconciledCfgMap.Data[UUIDCfgMapKey]
	// or update it
	if !ok {
		newUUID := k8suuid.NewUUID()
		if reconciledCfgMap.Data == nil {
			reconciledCfgMap.Data = map[string]string{}
		}
		reconciledCfgMap.Data[UUIDCfgMapKey] = string(newUUID)
		_, err := c.Update(ctx, reconciledCfgMap, metav1.UpdateOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return types.UID(""), err
		}

		return newUUID, nil
	}

	return types.UID(UUID), nil
}

// getDistribution returns the k8s distribution by fetching the GitVersion (legacy name) of the Info returned by ServerVersion().
func getDistribution(clientset kubernetes.Interface) (string, error) {
	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}

	return version.GitVersion, nil
}

// GetBuildInfo returns information about the current build.
func GetBuildInfo() BuildInfo {
	return BuildInfo{
		version,
		buildHash,
		buildDate,
		buildSnapshot,
	}
}
