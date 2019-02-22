// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"reflect"

	"github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	common "github.com/elastic/k8s-operators/operators/pkg/controller/common/user"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/secret"
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibana"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// InternalKibanaServerUserName is a user to be used by the Kibana server when interacting with ES.
	InternalKibanaServerUserName = "elastic-internal-kibana"
)

// name to identify the Kibana user object (secret/user CRD)
func kibanaUserObjectName(owner types.NamespacedName) string {
	return owner.Name + "-" + InternalKibanaServerUserName
}

func clearTextSecretKeySelector(assoc v1alpha1.KibanaElasticsearchAssociation) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: kibanaUserObjectName(k8s.ExtractNamespacedName(&assoc)),
		},
		Key: InternalKibanaServerUserName,
	}
}

func reconcileEsUser(c k8s.Client, s *runtime.Scheme, assoc v1alpha1.KibanaElasticsearchAssociation) error {

	// keep this name constant and bound to the association we cannot change it
	name := kibanaUserObjectName(k8s.ExtractNamespacedName(&assoc))
	pw := common.RandomPasswordBytes()
	expectedCreds := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: assoc.Spec.Kibana.Namespace,
			Labels:    kibana.NewLabels(assoc.Spec.Kibana.Name),
		},
		Data: map[string][]byte{
			InternalKibanaServerUserName: pw,
		},
	}

	reconciled := corev1.Secret{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &assoc,
		Expected:   &expectedCreds,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			_, ok := reconciled.Data[InternalKibanaServerUserName]
			//TODO compare labels and namespace and delete if necessary!
			return !ok

		},
		UpdateReconciled: func() {
			reconciled.Data = expectedCreds.Data
		},
	})
	expectedCreds.Data = reconciled.Data // make sure we don't constantly update the password
	if err != nil {
		return err
	}

	bcryptHash, err := bcrypt.GenerateFromPassword(expectedCreds.Data[InternalKibanaServerUserName], bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	expectedUser := &estype.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: assoc.Spec.Elasticsearch.Namespace,
			Labels:    label.NewLabels(assoc.Spec.Elasticsearch.NamespacedName()), //TODO add label for association
		},
		Spec: estype.UserSpec{
			Name:         InternalKibanaServerUserName,
			PasswordHash: string(bcryptHash),
			UserRoles:    []string{secret.KibanaUserBuiltinRole},
		},
	}

	reconciledUser := estype.User{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &assoc,
		Expected:   expectedUser,
		Reconciled: &reconciledUser,
		NeedsUpdate: func() bool {
			// TODO compare namespace or at least GC dangling users?
			return !reflect.DeepEqual(expectedUser.Spec, reconciledUser.Spec)
		},
		UpdateReconciled: func() {
			reconciledUser.Spec = expectedUser.Spec
		},
	})

}
