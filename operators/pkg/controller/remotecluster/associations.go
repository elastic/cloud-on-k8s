// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"reflect"

	assoctype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/associations/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
)

// associatedCluster represents a cluster that is part of an association in a RemoteCluster object.
// It contains an object selector and the CA, this is all we need to create an association.
// The CA reference can be nil if there is no CA for the given Selector.
type associatedCluster struct {
	Selector assoctype.ObjectSelector
	CA       []byte
}

// newAssociatedCluster creates an associatedCluster from an object selector.
func newAssociatedCluster(c k8s.Client, selector assoctype.ObjectSelector) (associatedCluster, error) {
	ca, err := nodecerts.GetCA(c, selector.NamespacedName())
	if err != nil {
		return associatedCluster{}, err
	}
	return associatedCluster{
		Selector: selector,
		CA:       ca,
	}, nil
}

// reconcileTrustRelationShip creates a TrustRelationShip from a local cluster to a remote one.
func reconcileTrustRelationShip(
	c k8s.Client,
	owner v1alpha1.RemoteCluster,
	name string,
	local, remote associatedCluster,
	subjectName []string,
) error {

	log.V(1).Info(
		"Reconcile TrustRelationShip",
		"name", name,
		"local-namespace", local.Selector.Namespace,
		"local-name", local.Selector.Name,
		"remote-namespace", remote.Selector.Namespace,
		"remote-name", remote.Selector.Name,
	)

	// Define the desired TrustRelationship object, it lives in the remote namespace.
	expected := v1alpha1.TrustRelationship{
		ObjectMeta: trustRelationshipObjectMeta(name, owner, local.Selector),
		Spec: v1alpha1.TrustRelationshipSpec{
			CaCert: string(remote.CA),
			TrustRestrictions: v1alpha1.TrustRestrictions{
				Trust: v1alpha1.Trust{
					SubjectName: subjectName,
				},
			},
		},
	}

	var reconciled v1alpha1.TrustRelationship
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme.Scheme,
		Owner:      &owner,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(reconciled.Spec, expected.Spec)
		},
		UpdateReconciled: func() {
			reconciled.Spec = expected.Spec
		},
	})
}

func ensureTrustRelationshipIsDeleted(
	c k8s.Client,
	name string,
	owner v1alpha1.RemoteCluster,
	cluster assoctype.ObjectSelector,
) error {
	trustRelationShip := &v1alpha1.TrustRelationship{}
	trustRelationShipObjectMeta := trustRelationshipObjectMeta(name, owner, cluster)
	log.Info("Deleting TrustRelationShip", "namespace", trustRelationShipObjectMeta.Namespace, "name", trustRelationShipObjectMeta.Name)
	err := c.Get(k8s.ExtractNamespacedName(&trustRelationShipObjectMeta), trustRelationShip)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return c.Delete(trustRelationShip)
}
