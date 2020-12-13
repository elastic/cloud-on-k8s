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
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func deleteOrphanedResources(
	ctx context.Context,
	c k8s.Client,
	info AssociationInfo,
	associated commonv1.Associated,
) error {
	span, _ := apm.StartSpan(ctx, "delete_orphaned_resources2", tracing.SpanTypeApp)
	defer span.End()

	assocKey := k8s.ExtractNamespacedName(associated)
	var associatedLabels client.MatchingLabels = info.AssociatedLabels(assocKey)

	// List all the Secrets involved in an association (users and ca)
	var secrets corev1.SecretList
	if err := c.List(&secrets, associatedLabels); err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		for _, association := range associated.GetAssociations() {
			if isSecretForAssociation(info, secret, association) {
				goto nextSecret
			}
		}

		// Secret for the `associated` resource doesn't match any `association` - it's not needed anymore and should be deleted.
		log.Info("Deleting secret", "namespace", secret.Namespace, "secret_name", secret.Name, "associated_name", associated.GetName())
		if err := c.Delete(&secret); err != nil && !apierrors.IsNotFound(err) {
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
