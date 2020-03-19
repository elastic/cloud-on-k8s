// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func entSearchWithConfigRef(secretNames ...string) entsv1beta1.EnterpriseSearch {
	entSearch := entsv1beta1.EnterpriseSearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "entsearch",
		},
	}
	for _, secretName := range secretNames {
		entSearch.Spec.ConfigRef = append(entSearch.Spec.ConfigRef, entsv1beta1.ConfigSource{
			SecretRef: commonv1.SecretRef{SecretName: secretName}})
	}
	return entSearch
}

func secretWithConfig(name string, cfg []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      name,
		},
		Data: map[string][]byte{
			ConfigFilename: cfg,
		},
	}
}

func Test_parseConfigRef(t *testing.T) {
	tests := []struct {
		name        string
		secrets     []runtime.Object
		ents        entsv1beta1.EnterpriseSearch
		wantConfig  *settings.CanonicalConfig
		wantWatches bool
		wantErr     bool
	}{
		{
			name:        "no configRef specified",
			secrets:     nil,
			ents:        entSearchWithConfigRef(),
			wantConfig:  settings.NewCanonicalConfig(),
			wantWatches: false,
		},
		{
			name: "merge entries from all secrets, priority to the last one",
			secrets: []runtime.Object{
				secretWithConfig("secret-1", []byte("a: b\nc: d")),
				secretWithConfig("secret-2", []byte("a: b-2\nc: d")),
				secretWithConfig("secret-3", []byte("a: b-3")),
			},
			ents: entSearchWithConfigRef("secret-1", "secret-2", "secret-3"),
			wantConfig: settings.MustCanonicalConfig(map[string]string{
				"a": "b-3",
				"c": "d",
			}),
			wantWatches: true,
		},
		{
			name: "a referenced secret does not exist: return an error",
			secrets: []runtime.Object{
				secretWithConfig("secret-1", []byte("a: b\nc: d")),
				secretWithConfig("secret-2", []byte("a: b-2\nc: d")),
			},
			ents:        entSearchWithConfigRef("secret-1", "secret-2", "secret-3"),
			wantConfig:  nil,
			wantWatches: true,
			wantErr:     true,
		},
		{
			name: "a referenced secret is invalid: return an error",
			secrets: []runtime.Object{
				secretWithConfig("secret-1", []byte("a: b\nc: d")),
				secretWithConfig("secret-2", []byte("a: b-2\nc: d")),
				secretWithConfig("secret-3", []byte("invalidyaml")),
			},
			ents:        entSearchWithConfigRef("secret-1", "secret-2", "secret-3"),
			wantConfig:  nil,
			wantWatches: true,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.secrets...)
			w := watches.NewDynamicWatches()
			got, err := parseConfigRef(c, w, tt.ents)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantConfig, got)
			if tt.wantWatches {
				require.Len(t, w.Secrets.Registrations(), 1)
			} else {
				require.Len(t, w.Secrets.Registrations(), 0)
			}
		})
	}
}
