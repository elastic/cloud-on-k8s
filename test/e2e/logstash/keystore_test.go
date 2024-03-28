// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	lsctrl "github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/logstash"
)

var (
	request = logstash.Request{
		Name: "pipeline [main]",
		Path: "/_node/stats/pipelines/main",
	}

	want = logstash.Want{
		Match: map[string]string{
			"pipelines.main.plugins.filters.0.events.out": "1",
		},
	}
)

// TestLogstashKeystoreWithoutPassword Logstash should resolve ${VAR} in pipelines.yml using keystore key value
// Logstash keystore variable cannot do string comparison in conditional statement.
// When variable is undefined in keystore, the pipeline fails, resulting in a test failure.
func TestLogstashKeystoreWithoutPassword(t *testing.T) {
	secretName := "ls-keystore-secure-settings"

	secureSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			"HELLO": "HALLO",
			"A":     "a",
			"B":     "b",
			"C":     "c",
		},
	}

	pipelineConfig := commonv1.Config{
		Data: map[string]interface{}{
			"pipeline.id": "main",
			"config.string": `
input { generator { count => 1 } } 
filter {
  mutate { add_tag => ["${A}", "${B}", "${C}"] }
}
`,
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create secret for keystore",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(secureSecret)
			}),
		})
	})

	b := logstash.NewBuilder("test-keystore-without-pw").
		WithNodeCount(1).
		WithSecureSettings(commonv1.SecretSource{SecretName: secretName}).
		WithPipelines([]commonv1.Config{pipelineConfig})

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			b.CheckMetricsRequest(k, request, want),
			test.Step{
				Name: "Delete secure secret",
				Test: test.Eventually(func() error {
					return k.DeleteSecrets(secureSecret)
				}),
			},
		}
	})

	test.Sequence(before, steps, b).RunSequential(t)
}

// TestLogstashKeystoreWithPassword Logstash with customized keystore password
// should resolve ${VAR} in pipelines.yml using keystore key value
func TestLogstashKeystoreWithPassword(t *testing.T) {
	secureSettingSecretName := "ls-keystore-pw-secure-settings"

	secureSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			"HELLO": "HALLO",
		},
	}

	passwordSecretName := "ls-keystore-pw"

	passwordSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      passwordSecretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			lsctrl.KeystorePassKey: "changed",
		},
	}

	pipelineConfig := commonv1.Config{
		Data: map[string]interface{}{
			"pipeline.id": "main",
			"config.string": `
input { generator { count => 1 } } 
filter {
  if ("${HELLO:}" != "") {
    mutate { add_tag => ["ok"] }
  }
}
`,
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create secret for keystore",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(secureSecret)
			}),
		}).WithStep(test.Step{
			Name: "Create secret for keystore password",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(passwordSecret)
			}),
		})
	})

	b := logstash.NewBuilder("test-keystore-with-default-pw").
		WithNodeCount(1).
		WithPipelines([]commonv1.Config{pipelineConfig}).
		WithSecureSettings(commonv1.SecretSource{SecretName: secureSettingSecretName}).
		WithPodTemplate(corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "logstash",
						Env: []corev1.EnvVar{
							{
								Name: lsctrl.KeystorePassKey,
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: passwordSecretName},
										Key:                  lsctrl.KeystorePassKey,
									},
								},
							},
						},
					},
				},
			},
		})

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			b.CheckMetricsRequest(k, request, want),
			test.Step{
				Name: "Delete secure secret",
				Test: test.Eventually(func() error {
					return k.DeleteSecrets(secureSecret)
				}),
			},
			test.Step{
				Name: "Delete keystore pw secret",
				Test: test.Eventually(func() error {
					return k.DeleteSecrets(passwordSecret)
				}),
			},
		}
	})

	test.Sequence(before, steps, b).RunSequential(t)
}
