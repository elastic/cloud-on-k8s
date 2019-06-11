// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package info

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

// Info contains versioning information.
type Info struct {
	UUID         types.UID `json:"uuid"`
	Version      Version   `json:"version"`
	Namespace    string    `json:"namespace"`
	Distribution string    `json:"distribution"`
}

func (i Info) IsDefined() bool {
	return i.Namespace != "" &&
		i.UUID != "" &&
		i.Distribution != "" &&
		i.Version.Number != "0.0.0" &&
		i.Version.BuildHash != "00000000" &&
		i.Version.BuildDate != "1970-01-01T00:00:00Z"
}

// Version contains number and build metadata information.
type Version struct {
	Number        string `json:"number"`
	BuildHash     string `json:"build_hash"`
	BuildDate     string `json:"build_date"`
	BuildSnapshot string `json:"build_snapshot"`
}

// New creates a new Info given a operator namespace and kubernetes client config.
func New(operatorNs string, cfg *rest.Config) Info {
	distribution, err := getDistribution(cfg)
	if err != nil {
		distribution = "unknown"
	}

	return Info{
		Version: Version{
			version,
			buildHash,
			buildDate,
			buildSnapshot,
		},
		Namespace:    operatorNs,
		Distribution: distribution,
	}
}

// ReconcileOperatorUUID reconciles a config map in the operator namespace whose the UID is used for the operator UUID.
func ReconcileOperatorUUID(c k8s.Client, s *runtime.Scheme, ns string) (types.UID, error) {
	var reconciledCfgMap corev1.ConfigMap
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client: c,
		Scheme: s,
		Expected: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
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
