// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

// isAutoOpsAssociationAllowed checks if the AutoOpsAgentPolicy is allowed to access the given Elasticsearch cluster.
// Access is allowed if:
// - The policy and ES cluster are in the same namespace, OR
// - The policy's service account has 'get' permission on the Elasticsearch resource in its namespace
func isAutoOpsAssociationAllowed(
	ctx context.Context,
	accessReviewer rbac.AccessReviewer,
	policy *autoopsv1alpha1.AutoOpsAgentPolicy,
	es *esv1.Elasticsearch,
	eventRecorder record.EventRecorder,
) (bool, error) {
	accessAllowed, err := accessReviewer.AccessAllowed(
		ctx,
		policy.Spec.ServiceAccountName,
		policy.Namespace,
		es,
	)
	if err != nil {
		return false, err
	}
	if !accessAllowed {
		logNotAllowedAssociation(ctx, policy, es, eventRecorder)
		return false, nil
	}
	return true, nil
}

func logNotAllowedAssociation(
	ctx context.Context,
	policy *autoopsv1alpha1.AutoOpsAgentPolicy,
	es *esv1.Elasticsearch,
	eventRecorder record.EventRecorder,
) {
	ulog.FromContext(ctx).Info("AutoOps policy not allowed to access Elasticsearch cluster",
		"service_account", policy.Spec.ServiceAccountName,
		"es_namespace", es.GetNamespace(),
		"es_name", es.GetName(),
	)
	eventRecorder.Eventf(
		policy,
		corev1.EventTypeWarning,
		events.EventAssociationError,
		"AutoOps policy not allowed to access Elasticsearch cluster: %s/%s to %s/%s",
		policy.Namespace, policy.Name, es.Namespace, es.Name,
	)
}

// requeueRbacCheck returns a reconcile result depending on the implementation of the AccessReviewer.
// When using the SubjectAccessReviewer, a next reconcile loop should be triggered later to keep the
// policy in sync with any RBAC role and binding changes.
func requeueRbacCheck(accessReviewer rbac.AccessReviewer) reconcile.Result {
	switch accessReviewer.(type) {
	case *rbac.SubjectAccessReviewer:
		return reconcile.Result{RequeueAfter: 15 * time.Minute}
	default:
		return reconcile.Result{}
	}
}
