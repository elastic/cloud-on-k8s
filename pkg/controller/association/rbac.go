// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

type Unbinder interface {
	Unbind(association commonv1.Association) error
}

// CheckAndUnbind checks if a reference is allowed and unbinds the association if it is not the case
func CheckAndUnbind(
	accessReviewer rbac.AccessReviewer,
	association commonv1.Association,
	referencedObject runtime.Object,
	unbinder Unbinder,
	eventRecorder record.EventRecorder,
) (bool, error) {
	allowed, err := accessReviewer.AccessAllowed(association.ServiceAccountName(), association.GetNamespace(), referencedObject)
	if err != nil {
		return false, err
	}
	if !allowed {
		metaObject, err := meta.Accessor(referencedObject)
		if err != nil {
			return false, nil
		}
		log.Info("Association not allowed",
			"associated_kind", association.GetObjectKind().GroupVersionKind().Kind,
			"associated_name", association.GetName(),
			"associated_namespace", association.GetNamespace(),
			"service_account", association.ServiceAccountName(),
			"remote_type", referencedObject.GetObjectKind().GroupVersionKind().Kind,
			"remote_namespace", metaObject.GetNamespace(),
			"remote_name", metaObject.GetName(),
		)
		eventRecorder.Eventf(
			association,
			corev1.EventTypeWarning,
			events.EventAssociationError,
			"Association not allowed: %s/%s to %s/%s",
			association.GetNamespace(), association.GetName(), metaObject.GetNamespace(), metaObject.GetName(),
		)
		return false, unbinder.Unbind(association)
	}
	return true, nil
}

// RequeueRbacCheck returns a reconcile result depending on the implementation of the AccessReviewer.
// It is mostly used when using the subjectAccessReviewer implementation in which case a next reconcile loop should be
// triggered later to keep the association in sync with the RBAC roles and bindings.
// See https://github.com/elastic/cloud-on-k8s/issues/2468#issuecomment-579157063
func RequeueRbacCheck(accessReviewer rbac.AccessReviewer) reconcile.Result {
	switch accessReviewer.(type) {
	case *rbac.SubjectAccessReviewer:
		return reconcile.Result{RequeueAfter: 15 * time.Minute}
	default:
		return reconcile.Result{}
	}
}
