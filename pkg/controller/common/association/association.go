// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

// ElasticsearchAuthSettings returns the user and the password to be used by an associated object to authenticate
// against an Elasticsearch cluster.
func ElasticsearchAuthSettings(
	c k8s.Client,
	associated commonv1.Associated,
) (username, password string, err error) {
	assocConf := associated.AssociationConf()
	if !assocConf.AuthIsConfigured() {
		return "", "", nil
	}

	secretObjKey := types.NamespacedName{Namespace: associated.GetNamespace(), Name: assocConf.AuthSecretName}
	var secret v1.Secret
	if err := c.Get(secretObjKey, &secret); err != nil {
		return "", "", err
	}
	return assocConf.AuthSecretKey, string(secret.Data[assocConf.AuthSecretKey]), nil
}

// IsConfiguredIfSet checks if an association is set in the spec and if it has been configured by an association controller.
// This is used to prevent the deployment of an associated resource while the association is not yet fully configured.
func IsConfiguredIfSet(associated commonv1.Associated, r record.EventRecorder) bool {
	esRef := associated.ElasticsearchRef()
	if (&esRef).IsDefined() && !associated.AssociationConf().IsConfigured() {
		r.Event(associated, v1.EventTypeWarning, events.EventAssociationError, "Elasticsearch backend is not configured")
		log.Info("Elasticsearch association not established: skipping associated resource deployment reconciliation",
			"kind", associated.GetObjectKind().GroupVersionKind().Kind,
			"namespace", associated.GetNamespace(),
			"name", associated.GetName(),
		)
		return false
	}
	return true
}

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
