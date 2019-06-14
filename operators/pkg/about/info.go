// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package about

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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
func NewOperatorInfo(operatorNs string, cfg *rest.Config) OperatorInfo {
	distribution, err := getDistribution(cfg)
	if err != nil {
		distribution = "unknown"
	}

	return OperatorInfo{
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

// ReconcileOperatorUUIDConfigMap reconciles a config map in the operator namespace whose the UID is used for the operator UUID.
func ReconcileOperatorUUIDConfigMap(operatorClient k8s.Client, s *runtime.Scheme, operatorNs string) (types.UID, error) {
	var reconciledCfgMap corev1.ConfigMap
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client: operatorClient,
		Scheme: s,
		Expected: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNs,
				Name:      UUIDCfgMapName,
			},
		},
		Reconciled:       &reconciledCfgMap,
		NeedsUpdate:      func() bool { return false },
		UpdateReconciled: func() {},
	}); err != nil {
		return types.UID(""), err
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
