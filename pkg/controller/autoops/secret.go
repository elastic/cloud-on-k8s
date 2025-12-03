// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// autoOpsESPasswordsSecretName is the name for the autoops-es-passwords Secret
	autoOpsESPasswordsSecretName = "autoops-es-passwords"
	// monitoringUserName is the name of the elastic-internal-monitoring user
	monitoringUserName = "elastic-internal-monitoring"
)

// encodeESSecretKey encodes a namespace/name combination into a valid secret key.
// Secret keys must match the regex '[-._a-zA-Z0-9]+', so we use base64 URL encoding
// which produces characters in that range.
func encodeESSecretKey(namespace, name string) string {
	key := fmt.Sprintf("%s/%s", namespace, name)
	return base64.URLEncoding.EncodeToString([]byte(key))
}

// reconcileAutoOpsESPasswordsSecret reconciles the Secret containing the elastic-internal-monitoring
// password for all Elasticsearch clusters referenced by the policy.
func reconcileAutoOpsESPasswordsSecret(
	ctx context.Context,
	c k8s.Client,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	esList []esv1.Elasticsearch,
) error {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconciling AutoOps ES passwords secret", "namespace", policy.Namespace)

	secretData := make(map[string][]byte)
	for _, es := range esList {
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			log.V(1).Info("Skipping ES cluster that is not ready", "namespace", es.Namespace, "name", es.Name)
			continue
		}

		internalUsersSecretKey := types.NamespacedName{
			Namespace: es.Namespace,
			Name:      esv1.InternalUsersSecret(es.Name),
		}
		var internalUsersSecret corev1.Secret
		if err := c.Get(ctx, internalUsersSecretKey, &internalUsersSecret); err != nil {
			if apierrors.IsNotFound(err) {
				log.V(1).Info("InternalUsersSecret not found for ES cluster, skipping", "namespace", es.Namespace, "name", es.Name)
				continue
			}
			return fmt.Errorf("while retrieving internal-users secret for ES cluster %s/%s: %w", es.Namespace, es.Name, err)
		}

		password, ok := internalUsersSecret.Data[monitoringUserName]
		if !ok {
			log.V(1).Info("elastic-internal-monitoring user not found in internal-users secret, skipping", "namespace", es.Namespace, "name", es.Name)
			continue
		}

		secretKey := encodeESSecretKey(es.Namespace, es.Name)
		secretData[secretKey] = password
	}

	expected := buildAutoOpsESPasswordsSecret(policy, secretData)

	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     c,
			Owner:      &policy,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
					!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
					!reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
				reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
				reconciled.Data = expected.Data
			},
		},
	)
}

// buildAutoOpsESPasswordsSecret builds the expected Secret for autoops ES passwords.
func buildAutoOpsESPasswordsSecret(policy autoopsv1alpha1.AutoOpsAgentPolicy, secretData map[string][]byte) corev1.Secret {
	meta := metadata.Propagate(&policy, metadata.Metadata{
		Labels:      policy.GetLabels(),
		Annotations: policy.GetAnnotations(),
	})

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        autoOpsESPasswordsSecretName,
			Namespace:   policy.GetNamespace(),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: secretData,
	}
}

const (
	// autoOpsESCASecretPrefix is the prefix for CA secrets created for each ES instance
	autoOpsESCASecretPrefix = "autoops-es-ca"
)

// reconcileAutoOpsESCASecret reconciles the Secret containing the CA certificate
// for a specific Elasticsearch cluster, copying it from the ES instance's http-ca-internal secret.
func reconcileAutoOpsESCASecret(
	ctx context.Context,
	c k8s.Client,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
) error {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconciling AutoOps ES CA secret", "namespace", policy.Namespace, "es_namespace", es.Namespace, "es_name", es.Name)

	if es.Status.Phase != esv1.ElasticsearchReadyPhase {
		log.V(1).Info("Skipping ES cluster that is not ready", "namespace", es.Namespace, "name", es.Name)
		return nil
	}

	// Get the source secret: {es-name}-es-http-ca-internal
	sourceSecretKey := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      fmt.Sprintf("%s-es-http-ca-internal", es.Name),
	}
	var sourceSecret corev1.Secret
	if err := c.Get(ctx, sourceSecretKey, &sourceSecret); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("ES http-ca-internal secret not found, skipping", "namespace", es.Namespace, "name", es.Name)
			return nil
		}
		return fmt.Errorf("while retrieving http-ca-internal secret for ES cluster %s/%s: %w", es.Namespace, es.Name, err)
	}

	// Extract tls.crt from the source secret
	caCert, ok := sourceSecret.Data["tls.crt"]
	if !ok || len(caCert) == 0 {
		log.V(1).Info("tls.crt not found in http-ca-internal secret, skipping", "namespace", es.Namespace, "name", es.Name)
		return nil
	}

	// Create secret name: {es-name}-{es-namespace}-es-ca
	secretName := fmt.Sprintf("%s-%s-%s", es.Name, es.Namespace, autoOpsESCASecretPrefix)
	expected := buildAutoOpsESCASecret(policy, es, secretName, caCert)

	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     c,
			Owner:      &policy,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !maps.IsSubset(expected.Labels, reconciled.Labels) ||
					!maps.IsSubset(expected.Annotations, reconciled.Annotations) ||
					!reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
				reconciled.Annotations = maps.Merge(reconciled.Annotations, expected.Annotations)
				reconciled.Data = expected.Data
			},
		},
	)
}

// buildAutoOpsESCASecret builds the expected Secret for autoops ES CA certificate.
func buildAutoOpsESCASecret(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch, secretName string, caCert []byte) corev1.Secret {
	meta := metadata.Propagate(&policy, metadata.Metadata{
		Labels:      policy.GetLabels(),
		Annotations: policy.GetAnnotations(),
	})

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        secretName,
			Namespace:   policy.GetNamespace(),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: map[string][]byte{
			"ca.crt": caCert,
		},
	}
}
