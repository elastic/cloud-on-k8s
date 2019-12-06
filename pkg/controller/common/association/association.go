// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

// ElasticsearchAuthSettings returns the user and the password to be used by an associated object to authenticate
// against an Elasticsearch cluster.
func ElasticsearchAuthSettings(
	c k8s.Client,
	associated commonv1beta1.Associated,
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

// IsConfigured checks if an association is set in the spec and if it has been configured by an association controller.
// This is used to prevent the deployment of an associated resource while the association is not yet fully configured.
func IsConfigured(associated commonv1beta1.Associated, r record.EventRecorder) bool {
	esRef := associated.ElasticsearchRef()
	if (&esRef).IsDefined() && !associated.AssociationConf().IsConfigured() {
		r.Event(associated, v1.EventTypeWarning, events.EventAssociationError, "Elasticsearch backend is not configured")
		log.Info("Elasticsearch association not established: skipping associated resource deployment reconciliation",
			"kind", associated.GetObjectKind().GroupVersionKind().Kind,
			"namespace", associated.GetNamespace(),
			"kibana_name", associated.GetName(),
		)
		return false
	}
	return true
}
