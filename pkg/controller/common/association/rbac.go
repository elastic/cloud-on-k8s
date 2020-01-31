// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"time"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Unbinder interface {
	Unbind(associated commonv1.Associated) error
}

// CheckAndUnbind checks if a reference is allowed and unbind the association if it is not the case
func CheckAndUnbind(
	accessReviewer rbac.AccessReviewer,
	associated commonv1.Associated,
	object runtime.Object,
	unbinder Unbinder,
) (bool, error) {
	allowed, err := accessReviewer.AccessAllowed(associated.ServiceAccountName(), associated.GetNamespace(), object)
	if err != nil {
		return false, err
	}
	if !allowed {
		metaObject, err := meta.Accessor(object)
		if err != nil {
			return false, nil
		}
		log.Info("Association not allowed",
			"associated_kind", associated.GetObjectKind().GroupVersionKind().Kind,
			"associated_name", associated.GetName(),
			"associated_namespace", associated.GetNamespace(),
			"serviceAccount", associated.ServiceAccountName(),
			"remote_type", object.GetObjectKind().GroupVersionKind().Kind,
			"remote_name", metaObject.GetNamespace(),
			"remote_namespace", metaObject.GetName(),
		)
		return false, unbinder.Unbind(associated)
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
