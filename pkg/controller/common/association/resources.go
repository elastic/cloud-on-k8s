// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteOrphanedResources deletes resources created by an association that are left over from previous reconciliation
// attempts. Common use case is an Elasticsearch reference in Kibana or APMServer spec that was removed.
func DeleteOrphanedResources(
	ctx context.Context,
	c k8s.Client,
	associated commonv1.Associated,
	matchLabels client.MatchingLabels,
) error {
	span, _ := apm.StartSpan(ctx, "delete_orphaned_resources", tracing.SpanTypeApp)
	defer span.End()

	// List all the Secrets involved in an association (users and ca)
	var secrets corev1.SecretList
	if err := c.List(&secrets, matchLabels); err != nil {
		return err
	}

	for _, s := range secrets.Items {
		if err := deleteIfOrphaned(c, &s, associated); err != nil {
			return err
		}
	}
	return nil
}

// deleteIfOrphaned deletes a Secret if it is not needed anymore, either because the association has been removed
// or the target namespace has changed.
func deleteIfOrphaned(
	c k8s.Client,
	secret *corev1.Secret,
	associated commonv1.Associated,
) error {
	esRef := associated.ElasticsearchRef().WithDefaultNamespace(associated.GetNamespace())

	// Secret should not exist if there is no ES referenced in the spec or if the resource is deleted
	if !esRef.IsDefined() {
		return deleteSecret(c, secret, associated)
	}

	// User secrets created in the Elasticsearch namespace are handled differently.
	// We need to check if the referenced namespace has changed in the Spec.
	// If a Secret is found in a namespace which is not the one referenced in the Spec then the secret should be deleted.
	if value, ok := secret.Labels[common.TypeLabelName]; ok && value == esuser.AssociatedUserType && esRef.Namespace != secret.Namespace {
		return deleteSecret(c, secret, associated)
	}

	return nil
}

func deleteSecret(c k8s.Client, secret *corev1.Secret, associated commonv1.Associated) error {
	log.Info("Deleting secret", "namespace", secret.Namespace, "secret_name", secret.Name, "associated_name", associated.GetName())
	if err := c.Delete(secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
