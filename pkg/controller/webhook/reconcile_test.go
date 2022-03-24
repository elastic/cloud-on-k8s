// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package webhook

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"k8s.io/client-go/kubernetes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
)

func TestParams_ReconcileResources(t *testing.T) {
	w := Params{
		Name:       "elastic-webhook.k8s.elastic.co",
		Namespace:  "elastic-system",
		SecretName: "elastic-webhook-server-cert",
		Rotation: certificates.RotationParams{
			Validity:     certificates.DefaultCertValidity,
			RotateBefore: certificates.DefaultRotateBefore,
		},
	}

	clientset :=
		fake.NewSimpleClientset(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "elastic-system",
					Name:      "elastic-webhook-server-cert",
				},
			},
			&v1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "elastic-webhook.k8s.elastic.co",
				},
				Webhooks: []v1.ValidatingWebhook{
					{
						Name: "elastic-es-validation-v1.k8s.elastic.co",
						ClientConfig: v1.WebhookClientConfig{
							Service: &v1.ServiceReference{Name: "elastic-webhook-server", Namespace: "elastic-system"},
						},
					},
				},
			},
		)

	clientset.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "admissionregistration.k8s.io/v1",
			APIResources: []metav1.APIResource{
				{Name: "admissionregistration.k8s.io", Namespaced: false, Kind: "APIGroup", Group: "admissionregistration.k8s.io", Version: "v1"},
			},
		},
	}

	// retrieve the current webhook configuration interface
	wh, err := w.NewAdmissionControllerInterface(context.Background(), clientset)
	if err != nil {
		t.Errorf("Params.NewAdmissionControllerInterface() error = %v", err)
	}

	if err := w.ReconcileResources(context.Background(), clientset, wh); err != nil {
		t.Errorf("Params.ReconcileResources() error = %v", err)
	}

	ctx := context.Background()
	// Secret and ValidatingWebhookConfiguration must have been filled with the certificates
	// retrieve current webhook server cert secret
	webhookServerSecret, err := clientset.CoreV1().Secrets(w.Namespace).Get(ctx, w.SecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(webhookServerSecret.Data))

	// retrieve the current webhook configuration
	webhookConfiguration, err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, w.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	caBundle := webhookConfiguration.Webhooks[0].ClientConfig.CABundle
	assert.True(t, len(caBundle) > 0)

	// Check that the cert in the secret has been signed by the caBundle
	verifyCertificates(t, caBundle, webhookServerSecret.Data["tls.crt"])

	// Delete the content of the secret, certificates should be recreated
	webhookServerSecret.Data = map[string][]byte{}
	_, err = clientset.CoreV1().Secrets(w.Namespace).Update(ctx, webhookServerSecret, metav1.UpdateOptions{})
	assert.NoError(t, err)
	webhookServerSecret, err = clientset.CoreV1().Secrets(w.Namespace).Get(ctx, w.SecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 0, len(webhookServerSecret.Data))

	if err := w.ReconcileResources(ctx, clientset, wh); err != nil {
		t.Errorf("Params.ReconcileResources() error = %v", err)
	}

	webhookServerSecret, err = clientset.CoreV1().Secrets(w.Namespace).Get(ctx, w.SecretName, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(webhookServerSecret.Data))

	// retrieve the new ca
	webhookConfiguration, err = clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, w.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	caBundle = webhookConfiguration.Webhooks[0].ClientConfig.CABundle
	// Check again that the cert in the secret has been signed by the caBundle
	verifyCertificates(t, caBundle, webhookServerSecret.Data["tls.crt"])
}

func verifyCertificates(t *testing.T, rootCert []byte, serverCert []byte) {
	t.Helper()
	ca := x509.NewCertPool()
	ok := ca.AppendCertsFromPEM(rootCert)
	assert.True(t, ok)
	block, _ := pem.Decode(serverCert)
	assert.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	assert.NoError(t, err)
	opts := x509.VerifyOptions{
		DNSName: "elastic-webhook-server.elastic-system.svc",
		Roots:   ca,
	}
	_, err = cert.Verify(opts)
	assert.NoError(t, err)
}

func TestUpdateOperatorPod(t *testing.T) {
	sampleAnnotations := map[string]string{"foo": "bar"}
	type args struct {
		pod            corev1.Pod
		clientset      kubernetes.Interface
		modifiedPod    string
		unmodifiedPods []string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "First test",
			args: args{
				clientset: fake.NewSimpleClientset(
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "elastic-system",
							Name:        "pod-1",
							Labels:      map[string]string{"control-plane": "elastic-operator"},
							Annotations: sampleAnnotations,
						},
					},
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "elastic-system",
							Name:      "pod-2",
							Labels:    map[string]string{"control-plane": "elastic-operator"},
						},
					},
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "elastic-system",
							Name:        "pod-3",
							Annotations: sampleAnnotations,
						},
					},
				),
				pod: corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "elastic-system",
						Name:      "pod-2",
						Labels:    map[string]string{"control-plane": "elastic-operator"},
					},
				},
				modifiedPod:    "pod-2",
				unmodifiedPods: []string{"pod-1", "pod-3"},
			},
		},
		{
			name: "Second test",
			args: args{
				clientset: fake.NewSimpleClientset(
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "elastic-system",
							Name:        "pod-1",
							Labels:      map[string]string{"control-plane": "elastic-operator"},
							Annotations: sampleAnnotations,
						},
					},
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "elastic-system",
							Name:        "pod-2",
							Labels:      map[string]string{"control-plane": "elastic-operator"},
							Annotations: sampleAnnotations,
						},
					},
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "elastic-system",
							Name:        "pod-3",
							Annotations: sampleAnnotations,
						},
					},
				),
				pod: corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "elastic-system",
						Name:        "pod-1",
						Labels:      map[string]string{"control-plane": "elastic-operator"},
						Annotations: map[string]string{UpdateAnnotation: time.Now().Add(-time.Second * 5).Format(time.RFC3339Nano)},
					},
				},
				modifiedPod:    "pod-1",
				unmodifiedPods: []string{"pod-2", "pod-3"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousValue, previousValueExists := tt.args.pod.Annotations[UpdateAnnotation]
			UpdateOperatorPod(context.Background(), tt.args.pod, tt.args.clientset)
			gotPod, err := tt.args.clientset.CoreV1().Pods("elastic-system").Get(context.Background(), tt.args.modifiedPod, metav1.GetOptions{})
			assert.NoError(t, err)
			assert.NotNil(t, gotPod.Annotations)
			newValue, exists := gotPod.Annotations[UpdateAnnotation]
			assert.True(t, exists)

			if previousValueExists {
				assert.True(t, newValue > previousValue)
			}

			// Check that other Pods have not been modified
			for _, podName := range tt.args.unmodifiedPods {
				gotOtherPod, err := tt.args.clientset.CoreV1().Pods("elastic-system").Get(context.Background(), podName, metav1.GetOptions{})
				assert.NoError(t, err)
				assert.Equal(t, sampleAnnotations, gotOtherPod.Annotations)
			}

		})
	}
}
