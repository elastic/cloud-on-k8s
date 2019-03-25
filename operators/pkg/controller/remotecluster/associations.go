// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"reflect"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

// associatedCluster represents a cluster that is part of an association in a RemoteCluster object.
// It contains an object selector and the secret that contains the CA, this is all we need to
// create an association. The CA reference can be nil if there is no CA for the given Selector.
type associatedCluster struct {
	Selector v1alpha1.ObjectSelector
	CA       *[]byte
}

// newAssociatedCluster creates an associatedCluster from an object selector.
func newAssociatedCluster(c k8s.Client, selector v1alpha1.ObjectSelector) (associatedCluster, error) {
	ca, err := getCA(c, selector.NamespacedName())
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
	owner *v1alpha1.RemoteCluster,
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
			CaCert: string(*remote.CA),
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
		Owner:      owner,
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
	owner *v1alpha1.RemoteCluster,
	cluster v1alpha1.ObjectSelector,
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

// getCA attempts to fetch the CA of a cluster.
func getCA(c k8s.Client, es types.NamespacedName) (*[]byte, error) {
	remoteCA := &v1.Secret{}
	remoteSecretNamespacedName := getCASecretNamespacedName(es)
	err := c.Get(remoteSecretNamespacedName, remoteCA)
	if err != nil {
		log.V(1).Info("Error while fetching remote CA", "error", err)
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	// Extract the CA from the secret
	caCert, exists := remoteCA.Data[certificates.CAFileName]
	if !exists {
		log.V(1).Info(
			"CA file not found in secret", "secret",
			remoteSecretNamespacedName, "file", certificates.CAFileName,
		)
		return nil, nil
	}
	return &caCert, nil
}

func getCASecretNamespacedName(es types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Name:      nodecerts.CACertSecretName(es.Name),
		Namespace: es.Namespace,
	}
}
