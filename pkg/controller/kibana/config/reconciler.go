// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"github.com/elastic/cloud-on-k8s/pkg/about"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"reflect"
)

// ReconcileConfigSecret reconciles the expected Kibana config secret for the given Kibana resource.
// This managed secret is mounted into each pod of the Kibana deployment.
func ReconcileConfigSecret(
	client k8s.Client,
	kb kbv1.Kibana,
	kbSettings CanonicalConfig,
	operatorInfo about.OperatorInfo,
) error {
	settingsYamlBytes, err := kbSettings.Render()
	if err != nil {
		return err
	}
	telemetryYamlBytes, err := getTelemetryYamlBytes(operatorInfo)
	if err != nil {
		return err
	}
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kb.Namespace,
			Name:      SecretName(kb),
			Labels: map[string]string{
				label.KibanaNameLabelName: kb.Name,
			},
		},
		Data: map[string][]byte{
			SettingsFilename:  settingsYamlBytes,
			telemetryFilename: telemetryYamlBytes,
		},
	}
	reconciled := corev1.Secret{}
	if err := reconciler.ReconcileResource(reconciler.Params{
		Client:     client,
		Scheme:     scheme.Scheme,
		Owner:      &kb,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(reconciled.Data, expected.Data)
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data
		},
	}); err != nil {
		return err
	}
	return nil
}

// GetConfig retrieves the canonical config for a given Kibana, if one exists
func GetConfig(client k8s.Client, kb v1beta1.Kibana) (*settings.CanonicalConfig, error) {
	var secret corev1.Secret
	var cfg *settings.CanonicalConfig
	err := client.Get(types.NamespacedName{Name: SecretName(kb), Namespace: kb.Namespace}, &secret)
	// how do we want to indicate that nothing is found?
	if err != nil && apierrors.IsNotFound(err) {
		return cfg, nil
	}
	rawCfg, exists := secret.Data[SettingsFilename]
	if !exists {
		// TODO make this an error
		return cfg, nil
	}
	// dict := make(map(string[interface{}]))
	// _ = yaml.Unmarshal(rawCfg, dict)
	// if err != nil{

	// }
	cfg, _ = settings.ParseConfig(rawCfg)
	return cfg, nil
}
