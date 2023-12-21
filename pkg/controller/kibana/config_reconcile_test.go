// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var defaultKibana = kbv1.Kibana{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "test-ns",
		Name:      "test",
	},
}

func TestReconcileConfigSecret(t *testing.T) {
	type args struct {
		initialObjects []client.Object
		kb             kbv1.Kibana
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
				initialObjects: []client.Object{&kbv1.Kibana{
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
				initialObjects: []client.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-kb-config",
							Namespace: "test-ns",
							Labels:    map[string]string{kblabel.KibanaNameLabelName: defaultKibana.Name},
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
				initialObjects: []client.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-kb-config",
							Namespace: "test-ns",
							Labels:    map[string]string{kblabel.KibanaNameLabelName: defaultKibana.Name},
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
			k8sClient := k8s.NewFakeClient(tt.args.initialObjects...)

			err := ReconcileConfigSecret(context.Background(), k8sClient, tt.args.kb, CanonicalConfig{settings.NewCanonicalConfig()})
			assert.NoError(t, err)

			var secrets corev1.SecretList
			labelSelector := client.MatchingLabels(map[string]string{kblabel.KibanaNameLabelName: tt.args.kb.Name})
			err = k8sClient.List(context.Background(), &secrets, labelSelector)
			assert.NoError(t, err)
			err = tt.assertions(secrets)
			assert.NoError(t, err)
		})
	}
}

func TestVersionDefaults(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		want    *settings.CanonicalConfig
	}{
		{
			name:    "6.x",
			version: "6.8.5",
			want:    settings.NewCanonicalConfig(),
		},
		{
			name:    "7.x",
			version: "7.1.0",
			want:    settings.NewCanonicalConfig(),
		},
		{
			name:    "7.6.0",
			version: "7.6.0",
			want: settings.MustCanonicalConfig(map[string]interface{}{
				XpackLicenseManagementUIEnabled: false,
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kb := &kbv1.Kibana{Spec: kbv1.KibanaSpec{Version: tc.version}}
			v := version.MustParse(tc.version)

			defaults := VersionDefaults(kb, v)
			var have map[string]interface{}
			require.NoError(t, defaults.Unpack(&have))

			var want map[string]interface{}
			require.NoError(t, tc.want.Unpack(&want))

			require.Equal(t, want, have)
		})
	}
}
