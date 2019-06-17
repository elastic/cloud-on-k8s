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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UUIDCfgMapName is the name of a config map whose the uuid is used for as an operator uuid.
// This operator UUID is then used in the OperatorInfo.
const UUIDCfgMapName = "elastic-operator-uuid"

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

// NewOperatorInfo creates a new OperatorInfo given a operator namespace and a Kubernetes client config.
func NewOperatorInfo(operatorUUID types.UID, operatorNs string, cfg *rest.Config) OperatorInfo {
	distribution, err := getDistribution(cfg)
	if err != nil {
		distribution = "unknown"
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
	}
}

// GetOperatorUUID returns the operator UUID by retrieving a config map or creating it if it does not exist.
func GetOperatorUUID(operatorClient client.Client, ns string) (types.UID, error) {
	c := k8s.WrapClient(operatorClient)
	// get the config map
	var reconciledCfgMap corev1.ConfigMap
	err := c.Get(types.NamespacedName{
		Namespace: ns,
		Name:      UUIDCfgMapName,
	}, &reconciledCfgMap)
	if err != nil && !apierrors.IsNotFound(err) {
		return types.UID(""), nil
	}

	// or create it
	if err != nil && apierrors.IsNotFound(err) {
		cfgMap := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      UUIDCfgMapName,
			}}
		err = c.Create(&cfgMap)
		if err != nil {
			return types.UID(""), nil
		}

		return cfgMap.UID, nil
	}

	return reconciledCfgMap.UID, nil
}

// getDistribution returns the k8s distribution by fetching the GitVersion (legacy name) of the Info returned by ServerVersion().
func getDistribution(cfg *rest.Config) (string, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	version, err := clientset.ServerVersion()
	if err != nil {
		return "", err
	}

	return version.GitVersion, nil
}
