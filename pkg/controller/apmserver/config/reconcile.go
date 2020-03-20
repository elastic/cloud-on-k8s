// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const ApmCfgSecretKey = "apm-server.yml"

var log = logf.Log.WithName("apmserver-config")

// Reconcile reconciles the configuration of the APM server: it first creates the configuration from the APM
// specification and then reconcile the underlying secret.
func Reconcile(client k8s.Client, as *apmv1.ApmServer) (corev1.Secret, error) {
	// Create a new configuration from the APM object spec.
	cfg, err := NewConfigFromSpec(client, as)
	if err != nil {
		return corev1.Secret{}, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return corev1.Secret{}, err
	}

	// Reconcile the configuration in a secret
	expectedConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			Name:      name.Config(as.Name),
			Labels:    labels.NewLabels(as.Name),
		},
		Data: map[string][]byte{
			ApmCfgSecretKey: cfgBytes,
		},
	}
	return reconciler.ReconcileSecret(client, expectedConfigSecret, as)
}
