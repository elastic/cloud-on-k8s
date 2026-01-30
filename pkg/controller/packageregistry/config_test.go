// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_newConfig(t *testing.T) {
	type args struct {
		runtimeObjs []client.Object
		epr         v1alpha1.PackageRegistry
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "no user config",
			args: args{
				runtimeObjs: nil,
				epr:         v1alpha1.PackageRegistry{},
			},
			want: `cache_time:
    catch_all: 10m
    categories: 10m
    index: 10s
    search: 10m
package_paths:
    - /packages/package-registry
    - /packages/package-storage
`,
			wantErr: false,
		},
		{
			name: "inline user config",
			args: args{
				runtimeObjs: nil,
				epr: v1alpha1.PackageRegistry{
					Spec: v1alpha1.PackageRegistrySpec{Config: &commonv1.Config{Data: map[string]any{
						"cache_time": map[string]any{
							"index": "11s",
						},
					}}},
				},
			},
			want: `cache_time:
    catch_all: 10m
    categories: 10m
    index: 11s
    search: 10m
package_paths:
    - /packages/package-registry
    - /packages/package-storage
`,
			wantErr: false,
		},
		{
			name: "with configRef",
			args: args{
				runtimeObjs: []client.Object{secretWithConfig("cfg", []byte("cache_time:\n  index: 11s"))},
				epr:         eprWithConfigRef("cfg", nil),
			},
			want: `cache_time:
    catch_all: 10m
    categories: 10m
    index: 11s
    search: 10m
package_paths:
    - /packages/package-registry
    - /packages/package-storage
`,
			wantErr: false,
		},
		{
			name: "configRef takes precedence",
			args: args{
				runtimeObjs: []client.Object{secretWithConfig("cfg", []byte("cache_time:\n  index: 20s"))},
				epr: eprWithConfigRef("cfg", &commonv1.Config{Data: map[string]any{
					"cache_time": map[string]any{
						"index": "50s",
					},
				}}),
			},
			want: `cache_time:
    catch_all: 10m
    categories: 10m
    index: 20s
    search: 10m
package_paths:
    - /packages/package-registry
    - /packages/package-storage
`,
			wantErr: false,
		},
		{
			name: "non existing configRef",
			args: args{
				epr: eprWithConfigRef("cfg", nil),
			},
			wantErr: true,
		},
		{
			name: "with unrelated secret (configRef should fail)",
			args: args{
				runtimeObjs: []client.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "sample-epr-user",
							Namespace: "ns",
						},
						Data: map[string][]byte{
							"ns-sample-epr-user": []byte("password"),
						},
					},
				},
				epr: eprWithConfigRef("cfg", nil),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ReconcilePackageRegistry{
				Client:         k8s.NewFakeClient(tt.args.runtimeObjs...),
				recorder:       record.NewFakeRecorder(10),
				dynamicWatches: watches.NewDynamicWatches(),
			}

			got, err := newConfig(&d, tt.args.epr)
			if (err != nil) != tt.wantErr {
				t.Errorf("newConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return // no point in checking the config contents
			}
			rendered, err := got.Render()
			require.NoError(t, err)
			if string(rendered) != tt.want {
				t.Errorf("newConfig() got = \n%v\n, want \n%v\n", string(rendered), tt.want)
			}
		})
	}
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

func eprWithConfigRef(name string, cfg *commonv1.Config) v1alpha1.PackageRegistry {
	return v1alpha1.PackageRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "epr",
			Namespace: "ns",
		},
		Spec: v1alpha1.PackageRegistrySpec{
			Config:    cfg,
			ConfigRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: name}}},
	}
}
