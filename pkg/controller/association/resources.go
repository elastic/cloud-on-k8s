// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. If a user changes namespace on a vertex of an association (eg. reference to an Elasticsearch resource in
// a different namespace) the standard reconcile mechanism will not delete the now redundant old user object/secret.
// This function lists all resources that don't match the current name/namespace combinations and deletes them.
func deleteOrphanedResources(
	ctx context.Context,
	c k8s.Client,
	info AssociationInfo,
	associated types.NamespacedName,
	associations []commonv1.Association,
) error {
	span, _ := apm.StartSpan(ctx, "delete_orphaned_resources", tracing.SpanTypeApp)
	defer span.End()

	var associatedLabels client.MatchingLabels = info.Labels(associated)

	// List all the Secrets involved in an association (users and ca)
	var secrets corev1.SecretList
	if err := c.List(context.Background(), &secrets, associatedLabels); err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		secret := secret
		for _, association := range associations {
			if isSecretForAssociation(info, secret, association) {
				goto nextSecret
			}
		}

		// Secret for the `associated` resource doesn't match any `association` - it's not needed anymore and should be deleted.
		log.Info("Deleting secret", "namespace", secret.Namespace, "secret_name", secret.Name, "associated_name", associated.Name)
		if err := c.Delete(context.Background(), &secret, &client.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &secret.UID}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}

	nextSecret:
	}

	return nil
}

func isSecretForAssociation(info AssociationInfo, secret corev1.Secret, association commonv1.Association) bool {
	ref := association.AssociationRef()

	// grab name from label (eg. elasticsearch.k8s.elastic.co/cluster-name=elasticsearch1 or kibana.k8s.elastic.co/name=kibana1)
	resourceName, ok := secret.Labels[info.AssociationResourceNameLabelName]
	if !ok || resourceName != ref.Name {
		// name points to a resource not involved in this `association`
		return false
	}

	// grab namespace from label (eg. elasticsearch.k8s.elastic.co/cluster-namespace=default or kibana.k8s.elastic.co/namespace=default)
	resourceNamespace, ok := secret.Labels[info.AssociationResourceNamespaceLabelName]
	if !ok || resourceNamespace != ref.Namespace {
		// namespace points to a resource not involved in this `association`
		return false
	}

	return true
}
