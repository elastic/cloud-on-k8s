// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"net"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	logstashcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/logstash"
)

// TestPipelineConfigRefLogstash PipelineRef should be able to take pipelines.yml from Secret.
func TestPipelineConfigRefLogstash(t *testing.T) {
	secretName := "ls-generator-pipeline"

	pipelineSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			"pipelines.yml": `
- pipeline.id: generator
  pipeline.workers: 1
  queue.drain: false
  config.string: input { generator {} } filter { sleep { time => 10 } } output { stdout { codec => dots } }
- pipeline.id: main
  config.string: input { stdin{} } output { stdout{} }`,
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create pipeline secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(pipelineSecret)
			}),
		})
	})

	name := "test-pipeline-ref"
	b := logstash.NewBuilder(name).
		WithNodeCount(1).
		WithPipelinesConfigRef(commonv1.ConfigMapOrSecretSource{
			SecretRef: commonv1.SecretRef{
				SecretName: secretName,
			},
		})

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			b.CheckMetricsRequest(k,
				logstash.Request{
					Name: "pipeline [generator]",
					Path: "/_node/pipelines/generator",
				},
				logstash.Want{
					Match: map[string]string{
						"pipelines.generator.workers": "1",
					},
					MatchFunc: map[string]func(string) bool{
						"status": isGreenOrYellow,
					},
				}),
			test.Step{
				Name: "Delete pipeline secret",
				Test: test.Eventually(func() error {
					return k.DeleteSecrets(pipelineSecret)
				}),
			},
		}
	})

	test.Sequence(before, steps, b).RunSequential(t)
}

// TestPipelineConfigMapRefLogstash PipelineRef should be able to take pipelines.yml from a ConfigMap.
func TestPipelineConfigMapRefLogstash(t *testing.T) {
	cmName := "ls-generator-pipeline-cm"

	pipelineCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Data: map[string]string{
			"pipelines.yml": `
- pipeline.id: generator
  pipeline.workers: 1
  queue.drain: false
  config.string: input { generator {} } filter { sleep { time => 10 } } output { stdout { codec => dots } }
- pipeline.id: main
  config.string: input { stdin{} } output { stdout{} }`,
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create pipeline configmap",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdate(&pipelineCM)
			}),
		})
	})

	name := "test-pipeline-cm-ref"
	b := logstash.NewBuilder(name).
		WithNodeCount(1).
		WithPipelinesConfigRef(commonv1.ConfigMapOrSecretSource{
			ConfigMapRef: commonv1.ConfigMapRef{
				ConfigMapName: cmName,
			},
		})

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			b.CheckMetricsRequest(k,
				logstash.Request{
					Name: "pipeline [generator]",
					Path: "/_node/pipelines/generator",
				},
				logstash.Want{
					Match: map[string]string{
						"pipelines.generator.workers": "1",
					},
					MatchFunc: map[string]func(string) bool{
						"status": isGreenOrYellow,
					},
				}),
			test.Step{
				Name: "Delete pipeline configmap",
				Test: test.Eventually(func() error {
					return k.Client.Delete(t.Context(), &pipelineCM)
				}),
			},
		}
	})

	test.Sequence(before, steps, b).RunSequential(t)
}

// TestPipelineConfigLogstash Pipeline should be able to pass to Logstash via VolumeMount.
func TestPipelineConfigLogstash(t *testing.T) {
	secretName := "ls-split-pipe"

	pipelineSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		StringData: map[string]string{
			"split.conf": "input { exec { command => \"uptime\" interval => 10 } } output { stdout{} }",
		},
	}

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create pipeline secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(pipelineSecret)
			}),
		})
	})

	name := "test-split-pipeline"
	volName := "ls-pipe-vol"
	mountPath := "/usr/share/logstash/pipeline"

	b := logstash.NewBuilder(name).
		WithNodeCount(1).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id": "split",
					"path.config": mountPath,
				},
			},
			{
				Data: map[string]interface{}{
					"pipeline.id":   "main",
					"config.string": "input { stdin{} } output { stdout{} }",
				},
			},
		}).
		WithVolumes(corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		}).
		WithVolumeMounts(corev1.VolumeMount{
			Name:      volName,
			MountPath: mountPath,
		})

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			b.CheckMetricsRequest(k,
				logstash.Request{
					Name: "pipeline [split]",
					Path: "/_node/pipelines/split",
				},
				logstash.Want{
					Match: map[string]string{
						"pipelines.split.batch_size": "125",
					},
					MatchFunc: map[string]func(string) bool{
						"status": isGreenOrYellow,
					},
				}),
			test.Step{
				Name: "Delete pipeline secret",
				Test: test.Eventually(func() error {
					return k.DeleteSecrets(pipelineSecret)
				}),
			},
		}
	})

	test.Sequence(before, steps, b).RunSequential(t)
}

// Verify that pipelines will reload when the Pipeline definition changes.
func TestLogstashPipelineReload(t *testing.T) {
	name := "test-ls-reload"

	logstashFirstPipeline := logstash.NewBuilder(name).WithNodeCount(1).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id":      "main",
					"pipeline.workers": 1,
					"config.string":    "input { beats{ port => 5044}} output { stdout{} }",
				},
			},
		})

	logstashSecondPipeline := logstash.Builder{Logstash: *logstashFirstPipeline.Logstash.DeepCopy()}.
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id":      "main",
					"pipeline.workers": 2,
					"config.string":    "input { beats{ port => 5044} } output { stdout{} }",
				},
			},
		}).
		WithMutatedFrom(&logstashFirstPipeline)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{}.
			WithSteps(logstashFirstPipeline.CheckK8sTestSteps(k)).
			WithStep(
				logstashFirstPipeline.CheckMetricsRequest(k,
					logstash.Request{
						Name: "pipeline [main]",
						Path: "/_node/pipelines/main",
					},
					logstash.Want{
						Match: map[string]string{
							"pipelines.main.workers": "1",
						},
						MatchFunc: map[string]func(string) bool{
							"status": isGreenOrYellow,
						},
					}),
			).
			WithSteps(logstashSecondPipeline.MutationTestSteps(k)).
			WithStep(
				logstashSecondPipeline.CheckMetricsRequest(k,
					logstash.Request{
						Name: "pipeline [main]",
						Path: "/_node/pipelines/main",
					},
					logstash.Want{
						Match: map[string]string{
							"pipelines.main.workers": "2",
						},
						MatchFunc: map[string]func(string) bool{
							"status": isGreenOrYellow,
						},
					}),
			)
	}

	test.Sequence(nil, stepsFn, logstashFirstPipeline).RunSequential(t)
}

// TestLogstashTLSCertReload verifies that when a TLS certificate Secret referenced by a Logstash pipeline
// is updated, Logstash automatically reloads the affected pipeline without restarting the pod.
// This relies on ssl.reload.automatic=true (set by ECK for Logstash >= 9.5.0) and
// config.reload.automatic=true (always set by ECK).
func TestLogstashTLSCertReload(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(logstashcontroller.MinSSLReloadVersion) {
		t.Skipf("ssl.reload.automatic requires Logstash >= %s, got %s", logstashcontroller.MinSSLReloadVersion.String(), v)
	}

	name := "test-ls-tls-rl"
	namespace := test.Ctx().ManagedNamespace(0)
	certSecretName := name + "-srv-cert"
	certMountPath := "/mnt/certs"

	cert1, key1 := helper.GenerateSelfSignedServerCertPKCS8(t, name)
	cert2, key2 := helper.GenerateSelfSignedServerCertPKCS8(t, name)

	certSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": cert1,
			"tls.key": key1,
		},
	}

	b := logstash.NewBuilder(name).
		WithNodeCount(1).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id": "main",
					// beats input with user-provided TLS cert. The Secret is mounted
					// without subPath so Kubernetes uses a projected volume with symlinks,
					// which is what ssl.reload.automatic's mtime-polling strategy detects.
					"config.string": fmt.Sprintf(
						`input { beats { port => 5044 ssl_enabled => true ssl_certificate => "%s/tls.crt" ssl_key => "%s/tls.key" } } output { stdout{} }`,
						certMountPath, certMountPath,
					),
				},
			},
		}).
		WithServices(logstashv1alpha1.LogstashService{
			Name: "beats",
			Service: commonv1.ServiceTemplate{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "beats", Port: 5044, Protocol: corev1.ProtocolTCP},
					},
				},
			},
		}).
		WithVolumes(corev1.Volume{
			Name: "srv-cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: certSecretName,
				},
			},
		}).
		WithVolumeMounts(corev1.VolumeMount{
			Name:      "srv-cert",
			MountPath: certMountPath,
			// no SubPath — Kubernetes projects the Secret as a directory of symlinks,
			// which is what ssl.reload.automatic's mtime-polling detects on rotation.
		})

	before := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{}.WithStep(test.Step{
			Name: "Create TLS cert secret",
			Test: test.Eventually(func() error {
				return k.CreateOrUpdateSecrets(certSecret)
			}),
		})
	})

	var podUIDs map[types.UID]struct{}

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			// Baseline: pipeline is healthy and has not reloaded yet.
			b.CheckMetricsRequest(k,
				logstash.Request{
					Name: "initial reload count is zero",
					Path: "/_node/stats/pipelines/main",
				},
				logstash.Want{
					Match: map[string]string{
						"pipelines.main.reloads.successes": "0",
					},
				},
			),
			// Snapshot pod UIDs so we can assert no restart occurred after reload.
			{
				Name: "Record Logstash pod UIDs before cert rotation",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(test.LogstashPodListOptions(namespace, b.Logstash.Name)...)
					if err != nil {
						return err
					}
					if len(pods) == 0 {
						return fmt.Errorf("no Logstash pods found")
					}
					podUIDs = make(map[types.UID]struct{}, len(pods))
					for _, pod := range pods {
						podUIDs[pod.UID] = struct{}{}
					}
					return nil
				}),
			},
			// Rotate: fetch the live Secret (to get its resourceVersion) then overwrite the data.
			{
				Name: "Rotate TLS certificate by updating the Secret",
				Test: test.Eventually(func() error {
					var current corev1.Secret
					if err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: namespace,
						Name:      certSecretName,
					}, &current); err != nil {
						return err
					}
					current.Data["tls.crt"] = cert2
					current.Data["tls.key"] = key2
					return k.Client.Update(context.Background(), &current)
				}),
			},
			// Logstash should detect the symlink mtime change and reload the pipeline.
			b.CheckMetricsRequest(k,
				logstash.Request{
					Name: "pipeline reloaded after cert rotation",
					Path: "/_node/stats/pipelines/main",
				},
				logstash.Want{
					MatchFunc: map[string]func(string) bool{
						"pipelines.main.reloads.successes": func(s string) bool {
							n, err := strconv.Atoi(s)
							return err == nil && n > 0
						},
					},
				},
			),
			// Open a raw TLS connection to the Beats port and verify the server presents cert2,
			// not cert1 — proving the reloaded pipeline is actually serving the rotated certificate.
			{
				Name: "rotated certificate is served by the Beats input",
				Test: test.Eventually(func() error {
					addr := fmt.Sprintf("%s.%s.svc:5044", logstashv1alpha1.UserServiceName(b.Logstash.Name, "beats"), namespace)
					var conn net.Conn
					var err error
					if test.Ctx().AutoPortForwarding {
						conn, err = portforward.NewForwardingDialer().DialContext(context.Background(), "tcp", addr)
					} else {
						conn, err = net.Dial("tcp", addr) //nolint:noctx
					}
					if err != nil {
						return fmt.Errorf("dial to Logstash beats input: %w", err)
					}
					defer conn.Close()
					tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
					if err := tlsConn.Handshake(); err != nil {
						return fmt.Errorf("TLS handshake with Logstash beats input: %w", err)
					}
					state := tlsConn.ConnectionState()
					if len(state.PeerCertificates) == 0 {
						return fmt.Errorf("beats input presented no certificates")
					}
					block, _ := pem.Decode(cert2)
					if block == nil {
						return fmt.Errorf("invalid cert2 PEM")
					}
					if !bytes.Equal(state.PeerCertificates[0].Raw, block.Bytes) {
						return fmt.Errorf("beats input is still serving old certificate (serial %s)", state.PeerCertificates[0].SerialNumber)
					}
					return nil
				}),
			},
			// The reload must not have triggered a pod restart.
			{
				Name: "Logstash pod should not have restarted after cert rotation",
				Test: func(t *testing.T) {
					pods, err := k.GetPods(test.LogstashPodListOptions(namespace, b.Logstash.Name)...)
					if err != nil {
						t.Fatalf("failed to list Logstash pods: %v", err)
					}
					for _, pod := range pods {
						if _, known := podUIDs[pod.UID]; !known {
							t.Errorf("pod %s (uid=%s) is new — unexpected restart during TLS cert reload", pod.Name, pod.UID)
						}
					}
				},
			},
			{
				Name: "Delete TLS cert secret",
				Test: test.Eventually(func() error {
					return k.DeleteSecrets(certSecret)
				}),
			},
		}
	})

	test.Sequence(before, steps, b).RunSequential(t)
}

// isGreenOrYellow returns true if the status is either green or yellow, red is considered as failure in health API.
func isGreenOrYellow(status string) bool {
	return status == "green" || status == "yellow"
}
