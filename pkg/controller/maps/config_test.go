// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_newConfig(t *testing.T) {
	type args struct {
		runtimeObjs []runtime.Object
		ems         v1alpha1.ElasticMapsServer
		ipFamily    corev1.IPFamily
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
				ems:         v1alpha1.ElasticMapsServer{},
				ipFamily:    corev1.IPv4Protocol,
			},
			want: `host: 0.0.0.0
ssl:
  certificate: /mnt/elastic-internal/http-certs/tls.crt
  enabled: true
  key: /mnt/elastic-internal/http-certs/tls.key
`,
			wantErr: false,
		},
		{
			name: "inline user config",
			args: args{
				runtimeObjs: nil,
				ems: v1alpha1.ElasticMapsServer{
					Spec: v1alpha1.MapsSpec{Config: &commonv1.Config{Data: map[string]interface{}{
						"ui": false,
					}}},
				},
				ipFamily: corev1.IPv4Protocol,
			},
			want: `host: 0.0.0.0
ssl:
  certificate: /mnt/elastic-internal/http-certs/tls.crt
  enabled: true
  key: /mnt/elastic-internal/http-certs/tls.key
ui: false
`,
			wantErr: false,
		},
		{
			name: "with configRef",
			args: args{
				runtimeObjs: []runtime.Object{secretWithConfig("cfg", []byte("ui: false"))},
				ems:         emsWithConfigRef("cfg", nil),
				ipFamily:    corev1.IPv4Protocol,
			},
			want: `host: 0.0.0.0
ssl:
  certificate: /mnt/elastic-internal/http-certs/tls.crt
  enabled: true
  key: /mnt/elastic-internal/http-certs/tls.key
ui: false
`,
			wantErr: false,
		},
		{
			name: "configRef takes precedence",
			args: args{
				runtimeObjs: []runtime.Object{secretWithConfig("cfg", []byte("ui: true"))},
				ems: emsWithConfigRef("cfg", &commonv1.Config{Data: map[string]interface{}{
					"ui": false,
				}}),
				ipFamily: corev1.IPv4Protocol,
			},
			want: `host: 0.0.0.0
ssl:
  certificate: /mnt/elastic-internal/http-certs/tls.crt
  enabled: true
  key: /mnt/elastic-internal/http-certs/tls.key
ui: true
`,
			wantErr: false,
		},
		{
			name: "non existing configRef",
			args: args{
				ems:      emsWithConfigRef("cfg", nil),
				ipFamily: corev1.IPv4Protocol,
			},
			wantErr: true,
		},
		{
			name: "non existing configRef",
			args: args{
				runtimeObjs: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "sample-maps-user",
							Namespace: "ns",
						},
						Data: map[string][]byte{
							"ns-sample-maps-user": []byte("password"),
						},
					},
				},
				ems: emsWithAssociation(commonv1.AssociationConf{
					AuthSecretName: "sample-maps-user",
					AuthSecretKey:  "ns-sample-maps-user",
					CACertProvided: true,
					CASecretName:   "sample-maps-es-ca",
					URL:            "https://elasticsearch-sample-es-http.default.svc:9200",
				}),
				ipFamily: corev1.IPv6Protocol,
			},
			want: `elasticsearch:
  host: https://elasticsearch-sample-es-http.default.svc:9200
  password: password
  ssl:
    certificateAuthorities: /mnt/elastic-internal/es-certs/ca.crt
    verificationMode: certificate
  username: ns-sample-maps-user
host: '::'
ssl:
  certificate: /mnt/elastic-internal/http-certs/tls.crt
  enabled: true
  key: /mnt/elastic-internal/http-certs/tls.key
`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ReconcileMapsServer{
				Client:         k8s.NewFakeClient(tt.args.runtimeObjs...),
				recorder:       record.NewFakeRecorder(10),
				dynamicWatches: watches.NewDynamicWatches(),
			}

			got, err := newConfig(&d, tt.args.ems, tt.args.ipFamily)
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

func emsWithConfigRef(name string, cfg *commonv1.Config) v1alpha1.ElasticMapsServer {
	return v1alpha1.ElasticMapsServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ems",
			Namespace: "ns",
		},
		Spec: v1alpha1.MapsSpec{
			Config:    cfg,
			ConfigRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: name}}},
	}
}

func emsWithAssociation(associationConf commonv1.AssociationConf) v1alpha1.ElasticMapsServer {
	ent := v1alpha1.ElasticMapsServer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "ems",
		},
	}
	ent.SetAssociationConf(&associationConf)
	return ent
}
