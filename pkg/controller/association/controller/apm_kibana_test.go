// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package controller

import (
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_getKibanaBasePath(t *testing.T) {
	type args struct {
		kb     v1.Kibana
		client k8s.Client
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Empty base path",
			args: args{
				kb:     getKibana(""),
				client: k8s.NewFakeClient(getKibanaConfigSecret(""), getKibanaDeployment("")),
			},
			want: "",
		},
		{
			name: "Base path set in Kibana spec",
			args: args{
				kb:     getKibana("/test"),
				client: k8s.NewFakeClient(getKibanaConfigSecret("/test"), getKibanaDeployment("")),
			},
			want: "/test",
		},
		{
			name: "Base path set in env var",
			args: args{
				kb:     getKibana(""),
				client: k8s.NewFakeClient(getKibanaConfigSecret(""), getKibanaDeployment("/monitoring")),
			},
			want: "/monitoring",
		},
		{
			name: "Base path set in env var preferred over secret",
			args: args{
				kb:     getKibana("/test"),
				client: k8s.NewFakeClient(getKibanaConfigSecret("/test"), getKibanaDeployment("/monitoring")),
			},
			want: "/monitoring",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getKibanaBasePath(tt.args.client, tt.args.kb)
			if err != nil {
				t.Errorf("getKibanaBasePath() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("getKibanaBasePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func getKibana(basePath string) v1.Kibana {
	kb := v1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kibana",
			Namespace: "ns",
		},
		Spec: v1.KibanaSpec{
			Config: &commonv1.Config{
				Data: map[string]interface{}{
					"test": map[string]interface{}{
						"testConfig": "testValue",
					},
				},
			},
		},
	}
	if basePath != "" {
		kb.Spec.Config.Data["server"] = map[string]interface{}{
			"basePath": basePath,
		}
	}

	return kb
}

func getKibanaConfigSecret(basePath string) client.Object {
	defaultConfig := []byte(`
test:
  testConfig: testValue
`)

	if basePath != "" {
		defaultConfig = []byte(fmt.Sprintf(`
test:
  testConfig: testValue
server:
  basePath: "%s"
`, basePath))
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kibana-kb-config",
			Namespace: "ns",
		},
		Data: map[string][]byte{
			"kibana.yml": defaultConfig,
		},
	}
}

func getKibanaDeployment(basePath string) client.Object {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kibana-kb",
			Namespace: "ns",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "kibana",
							Env: []corev1.EnvVar{
								{
									Name:  "SERVER_BASEPATH",
									Value: basePath,
								},
							},
						},
					},
				},
			},
		},
	}
}
