// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"errors"
	"reflect"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
)

// associatedCluster represents a cluster that is part of an association in a RemoteCluster object.
// It contains an object selector and the CA, this is all we need to create an association.
// The CA reference can be nil if there is no CA for the given Selector.
type associatedCluster struct {
	Selector commonv1alpha1.ObjectSelector
	CA       []byte
}

// newAssociatedCluster creates an associatedCluster from an object selector.
func newAssociatedCluster(c k8s.Client, selector commonv1alpha1.ObjectSelector) (associatedCluster, error) {
	var publicTransportCertsSecret v1.Secret
	if err := c.Get(
		transport.PublicCertsSecretRef(selector.NamespacedName()),
		&publicTransportCertsSecret,
	); err != nil {
		if apierrors.IsNotFound(err) {
			// public certs secret not created yet, return a nil CA for now
			return associatedCluster{
				Selector: selector,
			}, nil
		}
		return associatedCluster{}, err
	}

	if publicTransportCertsSecret.Data == nil {
		// public certs secret do not contain the CA yet, return a nil CA for now
		return associatedCluster{
			Selector: selector,
		}, nil
	}

	if caData, ok := publicTransportCertsSecret.Data[certificates.CAFileName]; ok {
		return associatedCluster{
			Selector: selector,
			CA:       caData,
		}, nil
	}

	return associatedCluster{}, errors.New("no ca file found in public transport certs secret")
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
	cluster commonv1alpha1.ObjectSelector,
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
