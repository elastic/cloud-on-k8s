// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	apiKeySecretType = "api-key"
	caSecretType     = "ca"
)

// reconcileAutoOpsESCASecret reconciles the Secret containing the CA certificate
// for a specific Elasticsearch cluster, copying it from the ES instance's http-certs-public secret.
func (r *AgentPolicyReconciler) reconcileAutoOpsESCASecret(
	ctx context.Context,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
) error {
	log := ulog.FromContext(ctx).WithValues("es_namespace", es.Namespace, "es_name", es.Name)
	log.V(1).Info("Reconciling AutoOps ES CA secret")

	if es.Status.Phase != esv1.ElasticsearchReadyPhase {
		log.V(1).Info("Skipping ES cluster that is not ready")
		return nil
	}

	sourceSecretKey := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
	}
	var sourceSecret corev1.Secret
	if err := r.Client.Get(ctx, sourceSecretKey, &sourceSecret); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("ES http-certs-public secret not found, skipping")
			return nil
		}
		return fmt.Errorf("while retrieving http-certs-public secret for ES cluster %s/%s: %w", es.Namespace, es.Name, err)
	}

	caCert, ok := sourceSecret.Data[certificates.CertFileName]
	if !ok || len(caCert) == 0 {
		log.V(1).Info("tls.crt not found in http-certs-public secret, skipping")
		return nil
	}

	secretName := autoopsv1alpha1.CASecret(policy.GetName(), es)
	expected := buildAutoOpsESCASecret(policy, es, secretName, caCert)

	reconciled := &corev1.Secret{}
	err := reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     r.Client,
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
	if err != nil {
		return err
	}

	watcher := k8s.ExtractNamespacedName(&policy)

	// Add a watch for the AutoOps CA secret
	return watches.WatchUserProvidedSecrets(
		watcher,
		r.dynamicWatches,
		secretName,
		[]string{secretName},
	)
}

// buildAutoOpsESCASecret builds the expected Secret for autoops ES CA certificate.
func buildAutoOpsESCASecret(policy autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch, secretName string, caCert []byte) corev1.Secret {
	if len(caCert) == 0 {
		return corev1.Secret{}
	}

	labels := resourceLabelsFor(policy, es)
	labels[policySecretTypeLabelKey] = caSecretType
	meta := metadata.Propagate(&policy, metadata.Metadata{
		Labels:      maps.Merge(policy.GetLabels(), labels),
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
			certificates.CAFileName: caCert,
		},
	}
}
