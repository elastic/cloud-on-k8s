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

const UUIDCfgMapName = "elastic-operator-uid"

var (
	// Variables that must be set during the manager initialization by calling `Setup(ns, cfg)`.
	operatorNamespace string
	k8sDistribution   string

	// Operator UUID set through a config map reconciliation.
	operatorUUID types.UID
)

// Info contains versioning information.
type Info struct {
	UUID         types.UID `json:"uuid"`
	Version      Version   `json:"version"`
	Namespace    string    `json:"namespace"`
	Distribution string    `json:"distribution"`
}

// Version contains number and build metadata information.
type Version struct {
	Number        string `json:"number"`
	BuildHash     string `json:"build_hash"`
	BuildDate     string `json:"build_date"`
	BuildSnapshot string `json:"build_snapshot"`
}

// Setup resolves the operator namespace and the k8s distribution.
func Setup(ns string, cfg *rest.Config) {
	operatorNamespace = ns
	k8sDistribution = getDistribution(cfg)
}

// Get returns operator information.
func Get() Info {
	return Info{
		operatorUUID,
		Version{
			version,
			buildHash,
			buildDate,
			buildSnapshot,
		},
		operatorNamespace,
		k8sDistribution,
	}
}

// ReconcileOperatorUUID reconciles a config map in the operator namespace whose the UID is used for the operator UUID.
func ReconcileOperatorUUID(c k8s.Client, s *runtime.Scheme) error {
	var reconciledCfgMap corev1.ConfigMap
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client: c,
		Scheme: s,
		Expected: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      UUIDCfgMapName,
			},
		},
		Reconciled:       &reconciledCfgMap,
		NeedsUpdate:      func() bool { return false },
		UpdateReconciled: func() {},
	}); err != nil {
		return err
	}

	operatorUUID = reconciledCfgMap.UID
	return nil
}

// getDistribution returns the k8s distribution by fetching the GitVersion (legacy name) of the Info returned by ServerVersion().
// It returns 'unknown' if an error occur.
func getDistribution(cfg *rest.Config) string {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "unknown"
	}
	version, err := clientset.ServerVersion()
	if err != nil {
		return "unknown"
	}

	return version.GitVersion
}
