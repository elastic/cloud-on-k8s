// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var defaultKibana = v1alpha1.Kibana{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "test-ns",
		Name:      "test",
	},
}

func TestReconcileConfigSecret(t *testing.T) {
	type args struct {
		initialObjects []runtime.Object
		kb             v1alpha1.Kibana
	}
	tests := []struct {
		name       string
		args       args
		assertions func(secrets corev1.SecretList) error
	}{
		{
			name: "config secret should be created",
			args: args{
				kb: defaultKibana,
				initialObjects: []runtime.Object{&v1alpha1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test-ns",
					},
				}},
			},
			assertions: func(secrets corev1.SecretList) error {
				require.Equal(t, 1, len(secrets.Items))
				assert.NotNil(t, secrets.Items[0].Data[SettingsFilename])
				return nil
			},
		},
		{
			name: "empty config secret should be updated",
			args: args{
				kb: defaultKibana,
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-kb-config",
							Namespace: "test-ns",
							Labels:    map[string]string{label.KibanaNameLabelName: defaultKibana.Name},
						},
						Data: map[string][]byte{},
					}},
			},

			assertions: func(secrets corev1.SecretList) error {
				require.Equal(t, 1, len(secrets.Items))
				assert.NotNil(t, secrets.Items[0].Data[SettingsFilename])
				return nil
			},
		},
		{
			name: "bad config secret should be updated",
			args: args{
				kb: defaultKibana,
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-kb-config",
							Namespace: "test-ns",
							Labels:    map[string]string{label.KibanaNameLabelName: defaultKibana.Name},
						},
						Data: map[string][]byte{
							SettingsFilename: []byte("eW8h"),
						},
					}},
			},

			assertions: func(secrets corev1.SecretList) error {
				require.Equal(t, 1, len(secrets.Items))
				assert.NotNil(t, secrets.Items[0].Data[SettingsFilename])
				assert.NotEqual(t, "eW8h", secrets.Items[0].Data[SettingsFilename])
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := scheme.Scheme
			if err := v1alpha1.SchemeBuilder.AddToScheme(sc); err != nil {
				assert.Fail(t, "failed to build custom scheme")
			}
			k8sClient := k8s.WrapClient(fake.NewFakeClientWithScheme(sc, tt.args.initialObjects...))

			err := ReconcileConfigSecret(k8sClient, tt.args.kb, CanonicalConfig{settings.NewCanonicalConfig()}, about.OperatorInfo{})
			assert.NoError(t, err)

			var secrets corev1.SecretList
			// TODO sabo are there funcs that can generate this, or also generate the labels in the test cases?
			labelSelector := client.MatchingLabels(map[string]string{label.KibanaNameLabelName: tt.args.kb.Name})
			err = k8sClient.List(&secrets, labelSelector)
			assert.NoError(t, err)
			err = tt.assertions(secrets)
			assert.NoError(t, err)
		})
	}
}
