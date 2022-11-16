// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remoteca

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

// isRemoteClusterAssociationAllowed checks if a bi-directional association is allowed between 2 clusters.
func isRemoteClusterAssociationAllowed(
	ctx context.Context,
	accessReviewer rbac.AccessReviewer,
	localEs, remoteEs *esv1.Elasticsearch,
	eventRecorder record.EventRecorder,
) (bool, error) {
	accessAllowed, err := accessReviewer.AccessAllowed(ctx, localEs.Spec.ServiceAccountName, localEs.Namespace, remoteEs)
	if err != nil {
		return false, err
	}
	if !accessAllowed {
		logNotAllowedAssociation(ctx, localEs, remoteEs, eventRecorder)
		return false, nil
	}
	accessAllowed, err = accessReviewer.AccessAllowed(ctx, remoteEs.Spec.ServiceAccountName, remoteEs.Namespace, localEs)
	if err != nil {
		return false, err
	}
	if !accessAllowed {
		logNotAllowedAssociation(ctx, remoteEs, localEs, eventRecorder)
		return false, nil
	}
	return true, nil
}

func logNotAllowedAssociation(ctx context.Context, localEs, remoteEs *esv1.Elasticsearch, eventRecorder record.EventRecorder) {
	ulog.FromContext(ctx).Info("Remote cluster association not allowed",
		"local_name", localEs.Name,
		"local_namespace", localEs.GetNamespace(),
		"service_account", localEs.Spec.ServiceAccountName,
		"remote_namespace", remoteEs.GetNamespace(),
		"remote_name", remoteEs.GetName(),
	)
	eventRecorder.Eventf(
		localEs,
		corev1.EventTypeWarning,
		events.EventAssociationError,
		"Remote cluster association not allowed: %s/%s to %s/%s",
		localEs.Namespace, localEs.Name, remoteEs.Namespace, remoteEs.Name,
	)
}
