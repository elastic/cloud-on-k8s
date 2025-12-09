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
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// autoOpsESCASecretSuffix is the suffix for CA secrets created for each ES instance
	autoOpsESCASecretSuffix = "autoops-es-ca" //nolint:gosec
)

// reconcileAutoOpsESCASecret reconciles the Secret containing the CA certificate
// for a specific Elasticsearch cluster, copying it from the ES instance's http-ca-internal secret.
func (r *ReconcileAutoOpsAgentPolicy) reconcileAutoOpsESCASecret(
	ctx context.Context,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	es esv1.Elasticsearch,
) error {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconciling AutoOps ES CA secret", "namespace", policy.Namespace, "es_namespace", es.Namespace, "es_name", es.Name)

	if es.Status.Phase != esv1.ElasticsearchReadyPhase {
		log.V(1).Info("Skipping ES cluster that is not ready", "namespace", es.Namespace, "name", es.Name)
		return nil
	}

	sourceSecretKey := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      certificates.CAInternalSecretName(esv1.ESNamer, es.Name, certificates.HTTPCAType),
	}
	var sourceSecret corev1.Secret
	if err := r.Client.Get(ctx, sourceSecretKey, &sourceSecret); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("ES http-ca-internal secret not found, skipping", "namespace", es.Namespace, "name", es.Name)
			return nil
		}
		return fmt.Errorf("while retrieving http-ca-internal secret for ES cluster %s/%s: %w", es.Namespace, es.Name, err)
	}

	caCert, ok := sourceSecret.Data[certificates.CertFileName]
	if !ok || len(caCert) == 0 {
		log.V(1).Info("tls.crt not found in http-ca-internal secret, skipping", "namespace", es.Namespace, "name", es.Name)
		return nil
	}

	secretName := fmt.Sprintf("%s-%s-%s", es.Name, es.Namespace, autoOpsESCASecretSuffix)
	expected := buildAutoOpsESCASecret(policy, secretName, caCert)

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

	watcher := types.NamespacedName{
		Name:      policy.Name,
		Namespace: policy.Namespace,
	}

	// Add a watch for the AutoOps CA secret
	return watches.WatchUserProvidedSecrets(
		watcher,
		r.dynamicWatches,
		secretName,
		[]string{secretName},
	)
}

// buildAutoOpsESCASecret builds the expected Secret for autoops ES CA certificate.
func buildAutoOpsESCASecret(policy autoopsv1alpha1.AutoOpsAgentPolicy, secretName string, caCert []byte) corev1.Secret {
	if len(caCert) == 0 {
		return corev1.Secret{}
	}

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
			certificates.CAFileName: caCert,
		},
	}
}
