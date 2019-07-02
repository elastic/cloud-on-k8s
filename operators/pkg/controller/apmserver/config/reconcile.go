// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const ApmCfgSecretKey = "apm-server.yml"

var log = logf.Log.WithName("apmserver-config")

// Reconcile reconciles the configuration of the APM server: it first creates the configuration from the APM
// specification and then reconcile the underlying secret.
func Reconcile(client k8s.Client, scheme *runtime.Scheme, as *v1alpha1.ApmServer) (*corev1.Secret, error) {

	// Create a new configuration from the APM object spec.
	cfg, err := NewConfigFromSpec(client, *as)
	if err != nil {
		return nil, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return nil, err
	}

	// Reconcile the configuration in a secret
	expectedConfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			Name:      name.Config(as.Name),
			Labels:    labels.NewLabels(as.Name),
		},
		Data: map[string][]byte{
			ApmCfgSecretKey: cfgBytes,
		},
	}

	reconciledConfigSecret := &corev1.Secret{}
	if err := reconciler.ReconcileResource(
		reconciler.Params{
			Client: client,
			Scheme: scheme,

			Owner:      as,
			Expected:   expectedConfigSecret,
			Reconciled: reconciledConfigSecret,

			NeedsUpdate: func() bool {
				return true
			},
			UpdateReconciled: func() {
				reconciledConfigSecret.Labels = expectedConfigSecret.Labels
				reconciledConfigSecret.Data = expectedConfigSecret.Data
			},
			PreCreate: func() {
				log.Info("Creating config secret", "name", expectedConfigSecret.Name)
			},
			PreUpdate: func() {
				log.Info("Updating config secret", "name", expectedConfigSecret.Name)
			},
		},
	); err != nil {
		return nil, err
	}
	return reconciledConfigSecret, nil
}
