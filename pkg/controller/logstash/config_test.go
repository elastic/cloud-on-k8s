// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_newConfig(t *testing.T) {
	type args struct {
		runtimeObjs []client.Object
		logstash    v1alpha1.Logstash
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
				logstash:    v1alpha1.Logstash{},
			},
			want: `api:
    http:
        host: 0.0.0.0
config:
    reload:
        automatic: true
`,
			wantErr: false,
		},
		{
			name: "inline user config",
			args: args{
				runtimeObjs: nil,
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{Config: &commonv1.Config{Data: map[string]interface{}{
						"log.level": "debug",
					}}},
				},
			},
			want: `api:
    http:
        host: 0.0.0.0
config:
    reload:
        automatic: true
log:
    level: debug
`,
			wantErr: false,
		},
		{
			name: "with configRef",
			args: args{
				runtimeObjs: []client.Object{secretWithConfig("cfg", []byte("log.level: debug"))},
				logstash:    logstashWithConfigRef("cfg", nil),
			},
			want: `api:
    http:
        host: 0.0.0.0
config:
    reload:
        automatic: true
log:
    level: debug
`,
			wantErr: false,
		},
		{
			name: "config takes precedence",
			args: args{
				runtimeObjs: []client.Object{secretWithConfig("cfg", []byte("log.level: debug"))},
				logstash: logstashWithConfigRef("cfg", &commonv1.Config{Data: map[string]interface{}{
					"log.level": "warn",
				}}),
			},
			want: `api:
    http:
        host: 0.0.0.0
config:
    reload:
        automatic: true
log:
    level: warn
`,
			wantErr: false,
		},
		{
			name: "non existing configRef",
			args: args{
				logstash: logstashWithConfigRef("cfg", nil),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := Params{
				Context:       context.Background(),
				Client:        k8s.NewFakeClient(tt.args.runtimeObjs...),
				EventRecorder: record.NewFakeRecorder(10),
				Watches:       watches.NewDynamicWatches(),
				Logstash:      tt.args.logstash,
			}

			got, err := buildConfig(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("newConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return // no point in checking the config contents
			}
			require.NoError(t, err)
			if string(got) != tt.want {
				t.Errorf("newConfig() got = \n%v\n, want \n%v\n", string(got), tt.want)
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
			LogstashConfigFileName: cfg,
		},
	}
}

func logstashWithConfigRef(name string, cfg *commonv1.Config) v1alpha1.Logstash {
	return v1alpha1.Logstash{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ls",
			Namespace: "ns",
		},
		Spec: v1alpha1.LogstashSpec{
			Config:    cfg,
			ConfigRef: &commonv1.ConfigSource{SecretRef: commonv1.SecretRef{SecretName: name}}},
	}
}
