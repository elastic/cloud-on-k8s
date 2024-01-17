// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/configs"
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
    ssl:
        enabled: true
        keystore:
            password: changeit
            path: /usr/share/logstash/config/api_keystore.p12
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
						"log.level":                 "debug",
						"api.ssl.keystore.password": "Str0ngP@ssw0rd",
					}}},
				},
			},
			want: `api:
    http:
        host: 0.0.0.0
    ssl:
        enabled: true
        keystore:
            password: Str0ngP@ssw0rd
            path: /usr/share/logstash/config/api_keystore.p12
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
    ssl:
        enabled: true
        keystore:
            password: changeit
            path: /usr/share/logstash/config/api_keystore.p12
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
    ssl:
        enabled: true
        keystore:
            password: changeit
            path: /usr/share/logstash/config/api_keystore.p12
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
		{
			name: "logstash config disables TLS and service disables TLS",
			args: args{
				runtimeObjs: nil,
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{
						Config: &commonv1.Config{Data: map[string]interface{}{
							"api.ssl.enabled": "false",
						}},
						Services: []v1alpha1.LogstashService{{
							Name: LogstashAPIServiceName,
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						}},
					},
				},
			},
			want: `api:
    http:
        host: 0.0.0.0
    ssl:
        enabled: "false"
config:
    reload:
        automatic: true
`,
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

			got, err := buildConfig(params, tt.args.logstash.APIServerTLSOptions().Enabled())
			if (err != nil) != tt.wantErr {
				t.Errorf("newConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // no point in checking the config contents
			}
			require.NoError(t, err)

			gotBytes, _ := got.Render()
			gotStr := string(gotBytes)
			if gotStr != tt.want {
				t.Errorf("newConfig() got = \n%v\n, want \n%v\n", gotStr, tt.want)
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
			ConfigFileName: cfg,
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

func Test_checkTLSConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  configs.APIServer
		useTLS  bool
		wantErr bool
	}{
		{
			name: "both svc and config enable TLS",
			config: configs.APIServer{
				SSLEnabled: "true",
			},
			useTLS:  true,
			wantErr: false,
		},
		{
			name: "both svc and config disable TLS",
			config: configs.APIServer{
				SSLEnabled: "false",
			},
			useTLS:  false,
			wantErr: false,
		},
		{
			name:    "svc disable TLS and config is unset",
			config:  configs.APIServer{},
			useTLS:  false,
			wantErr: false,
		},
		{
			name: "svc disable TLS but config enable TLS",
			config: configs.APIServer{
				SSLEnabled: "true",
			},
			useTLS:  false,
			wantErr: true,
		},
		{
			name: "svc enable TLS but config disable TLS",
			config: configs.APIServer{
				SSLEnabled: "false",
			},
			useTLS:  true,
			wantErr: true,
		},
		{
			name:    "svc enable TLS but config is unset",
			config:  configs.APIServer{},
			useTLS:  true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTLSConfig(tt.config, tt.useTLS)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // no point in checking the config contents
			}
		})
	}
}

func Test_resolveAPIServerConfig(t *testing.T) {
	secureSecretName := "logstash-secure-settings"
	secureSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secureSecretName},
		Data: map[string][]byte{
			"SSL_ENABLED":           []byte("true"),
			"SSL_KEYSTORE_PASSWORD": []byte("whatever"),
			"API_AUTH_TYPE":         []byte("basic"),
			"API_USERNAME":          []byte("batman"),
			"API_PASSWORD":          []byte("i_am_rich"),
		},
	}

	envFromSecretName := "logstash-env-secret" // #nosec G101
	envFromSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: envFromSecretName},
		Data: map[string][]byte{
			"SSL_ENABLED":           []byte("true"),
			"SSL_KEYSTORE_PASSWORD": []byte("whatever?"),
			"API_AUTH_TYPE":         []byte("basic"),
			"API_USERNAME":          []byte("superman"),
			"API_PASSWORD":          []byte("i_am_handsome"),
		},
	}

	envFromConfigMapName := "logstash-env-configmap"
	envFromConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: envFromConfigMapName},
		Data: map[string]string{
			"SSL_ENABLED":           "true",
			"SSL_KEYSTORE_PASSWORD": "whatever!",
			"API_AUTH_TYPE":         "basic",
			"API_USERNAME":          "spider-man",
			"API_PASSWORD":          "i_am_poor",
		},
	}

	config := &commonv1.Config{Data: map[string]interface{}{
		"api.ssl.enabled":           "${SSL_ENABLED}",
		"api.ssl.keystore.password": "${SSL_KEYSTORE_PASSWORD}",
		"api.auth.type":             "${API_AUTH_TYPE}",
		"api.auth.basic.username":   "${API_USERNAME}",
		"api.auth.basic.password":   "${API_PASSWORD}",
	}}

	type args struct {
		runtimeObjs []client.Object
		logstash    v1alpha1.Logstash
	}
	tests := []struct {
		name    string
		args    args
		want    configs.APIServer
		wantErr bool
	}{
		{
			name: "no user config",
			args: args{
				runtimeObjs: nil,
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{
						Config: &commonv1.Config{Data: map[string]interface{}{}},
					},
				},
			},
			want: configs.APIServer{
				SSLEnabled:       "true",
				KeystorePassword: APIKeystoreDefaultPass,
				AuthType:         "",
				Username:         "",
				Password:         "",
			},
			wantErr: false,
		},
		{
			name: "resolve variable from env",
			args: args{
				runtimeObjs: nil,
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{
						Config: config,
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "logstash",
										Env: []corev1.EnvVar{
											{
												Name:  "SSL_ENABLED",
												Value: "true",
											},
											{
												Name:  "SSL_KEYSTORE_PASSWORD",
												Value: "whatever",
											},
											{
												Name:  "API_AUTH_TYPE",
												Value: "basic",
											},
											{
												Name:  "API_USERNAME",
												Value: "batman",
											},
											{
												Name:  "API_PASSWORD",
												Value: "i_am_rich",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: configs.APIServer{
				SSLEnabled:       "true",
				KeystorePassword: "whatever",
				AuthType:         "basic",
				Username:         "batman",
				Password:         "i_am_rich",
			},
			wantErr: false,
		},
		{
			name: "resolve variable from secure settings secret when both secure settings and env declare the same key",
			args: args{
				runtimeObjs: []client.Object{&secureSecret, &envFromSecret, &envFromConfigMap},
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{
						Config: config,
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: secureSecretName,
							},
						},
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "logstash",
										Env: []corev1.EnvVar{
											{
												Name:  "API_USERNAME",
												Value: "ant-man",
											},
											{
												Name:  "API_PASSWORD",
												Value: "i_am_tiny",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: configs.APIServer{
				SSLEnabled:       "true",
				KeystorePassword: "whatever",
				AuthType:         "basic",
				Username:         "batman",
				Password:         "i_am_rich",
			},
			wantErr: false,
		},
		{
			name: "resolve variable from env config map",
			args: args{
				runtimeObjs: []client.Object{&secureSecret, &envFromSecret, &envFromConfigMap},
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{
						Config: config,
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "logstash",
										EnvFrom: []corev1.EnvFromSource{
											{
												ConfigMapRef: &corev1.ConfigMapEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: envFromConfigMapName,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: configs.APIServer{
				SSLEnabled:       "true",
				KeystorePassword: "whatever!",
				AuthType:         "basic",
				Username:         "spider-man",
				Password:         "i_am_poor",
			},
			wantErr: false,
		},
		{
			name: "resolve variable from env secret",
			args: args{
				runtimeObjs: []client.Object{&secureSecret, &envFromSecret, &envFromConfigMap},
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{
						Config: config,
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "logstash",
										EnvFrom: []corev1.EnvFromSource{
											{
												SecretRef: &corev1.SecretEnvSource{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: envFromSecretName,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: configs.APIServer{
				SSLEnabled:       "true",
				KeystorePassword: "whatever?",
				AuthType:         "basic",
				Username:         "superman",
				Password:         "i_am_handsome",
			},
			wantErr: false,
		},
		{
			name: "fails when secret doesn't exist",
			args: args{
				runtimeObjs: []client.Object{&secureSecret, &envFromSecret, &envFromConfigMap},
				logstash: v1alpha1.Logstash{
					Spec: v1alpha1.LogstashSpec{
						Config: config,
						PodTemplate: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{Name: "logstash"}},
							},
						},
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "non-exist-secret",
							},
						},
					},
				},
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

			cfg, err := buildConfig(params, tt.args.logstash.APIServerTLSOptions().Enabled())
			if err != nil {
				t.Errorf("buildConfig() error = %v", err)
				return
			}

			got, err := resolveAPIServerConfig(cfg, params)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveAPIServerConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // no point in checking the config contents
			}
			require.NoError(t, err)

			assert.Equal(t, tt.want.SSLEnabled, got.SSLEnabled)
			assert.Equal(t, tt.want.KeystorePassword, got.KeystorePassword)
			assert.Equal(t, tt.want.AuthType, got.AuthType)
			assert.Equal(t, tt.want.Username, got.Username)
			assert.Equal(t, tt.want.Password, got.Password)
		})
	}
}
