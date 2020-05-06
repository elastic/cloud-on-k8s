// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. If a user changes namespace on a vertex of an association (eg. reference to an Elasticsearch resource in
// a different namespace) the standard reconcile mechanism will not delete the now redundant old user object/secret.
// This function lists all resources that don't match the current name/namespace combinations and deletes them.
func deleteOrphanedResources(
	ctx context.Context,
	c k8s.Client,
	esRef commonv1.ObjectSelector,
	association commonv1.Association,
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
		if err := deleteIfOrphaned(c, &s, esRef, association); err != nil {
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
	esRef commonv1.ObjectSelector,
	association commonv1.Association,
) error {
	// Secret should not exist if there is no service referenced in the spec or if the resource is deleted
	serviceRef := association.AssociationRef().WithDefaultNamespace(association.GetNamespace())
	if !serviceRef.IsDefined() {
		return deleteSecret(c, secret, association)
	}

	// User secrets created in the Elasticsearch namespace are handled differently.
	// We need to check if the referenced namespace has changed in the Spec.
	// If a Secret is found in a namespace which is not the one referenced in the Spec then the secret should be deleted.
	if value, ok := secret.Labels[common.TypeLabelName]; ok &&
		value == esuser.AssociatedUserType &&
		(!esRef.IsDefined() || esRef.Namespace != secret.Namespace) {
		return deleteSecret(c, secret, association)
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
