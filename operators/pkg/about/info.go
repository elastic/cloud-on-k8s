// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package about

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8suuid "k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// UUIDCfgMapName is the name for the config map  containing the operator UUID.
	// The operator UUID is then used in the OperatorInfo.
	UUIDCfgMapName = "elastic-operator-uuid"
	// UUIDCfgMapName is used for the operator UUID inside a config map.
	UUIDCfgMapKey = "uuid"
)

// OperatorInfo contains information about the operator.
type OperatorInfo struct {
	UUID         types.UID `json:"uuid"`
	Namespace    string    `json:"namespace"`
	Distribution string    `json:"distribution"`
	BuildInfo    BuildInfo `json:"build"`
}

// BuildInfo contains build metadata information.
type BuildInfo struct {
	Version  string `json:"version"`
	Hash     string `json:"hash"`
	Date     string `json:"date"`
	Snapshot string `json:"snapshot"`
}

// IsDefined returns true if the info's default values have been replaced.
func (i OperatorInfo) IsDefined() bool {
	return i.UUID != "" &&
		i.Namespace != "" &&
		i.Distribution != "" &&
		i.BuildInfo.Version != "0.0.0" &&
		i.BuildInfo.Hash != "00000000" &&
		i.BuildInfo.Date != "1970-01-01T00:00:00Z"
}

// GetOperatorInfo returns an OperatorInfo given an operator client, a Kubernetes client config and an operator namespace.
func GetOperatorInfo(operatorClient client.Client, clientset kubernetes.Interface, operatorNs string) (OperatorInfo, error) {
	operatorUUID, err := getOperatorUUID(operatorClient, operatorNs)
	if err != nil {
		return OperatorInfo{}, err
	}

	distribution, err := getDistribution(clientset)
	if err != nil {
		return OperatorInfo{}, err
	}

	return OperatorInfo{
		UUID:         operatorUUID,
		Namespace:    operatorNs,
		Distribution: distribution,
		BuildInfo: BuildInfo{
			version,
			buildHash,
			buildDate,
			buildSnapshot,
		},
	}, nil
}

// getOperatorUUID returns the operator UUID by retrieving a config map or creating it if it does not exist.
func getOperatorUUID(operatorClient client.Client, operatorNs string) (types.UID, error) {
	c := k8s.WrapClient(operatorClient)
	// get the config map
	var reconciledCfgMap corev1.ConfigMap
	err := c.Get(types.NamespacedName{
		Namespace: operatorNs,
		Name:      UUIDCfgMapName,
	}, &reconciledCfgMap)
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
		err = c.Create(&cfgMap)
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
		err := c.Update(&reconciledCfgMap)
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
