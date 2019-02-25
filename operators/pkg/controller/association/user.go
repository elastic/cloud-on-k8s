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
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/user"
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
func kibanaUserObjectName(assocName string) string {
	return assocName + "-" + InternalKibanaServerUserName
}

func userKey(assoc v1alpha1.KibanaElasticsearchAssociation) types.NamespacedName {
	return types.NamespacedName{
		Namespace: assoc.Spec.Elasticsearch.Namespace,
		Name:      kibanaUserObjectName(assoc.Name),
	}
}

func secretKey(assoc v1alpha1.KibanaElasticsearchAssociation) types.NamespacedName {
	return types.NamespacedName{
		Namespace: assoc.Spec.Kibana.Namespace,
		Name:      kibanaUserObjectName(assoc.Name),
	}
}

// creates a SecretKeySelector selecting the Kibana user secret for the given association
func clearTextSecretKeySelector(assoc v1alpha1.KibanaElasticsearchAssociation) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: kibanaUserObjectName(assoc.Name),
		},
		Key: InternalKibanaServerUserName,
	}
}

// reconcileEsUser creates a User resources and a corresponding secret or updates those as appropriate.
func reconcileEsUser(c k8s.Client, s *runtime.Scheme, assoc v1alpha1.KibanaElasticsearchAssociation) error {
	// keep this name constant and bound to the association we cannot change it

	pw := common.RandomPasswordBytes()
	secretLabels := kibana.NewLabels(assoc.Spec.Kibana.Name)
	secretLabels[AssociationLabelName] = assoc.Name
	secKey := secretKey(assoc)
	expectedCreds := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secKey.Name,
			Namespace: secKey.Namespace,
			Labels:    secretLabels,
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
			//TODO compare labels
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

	userLabels := label.NewLabels(assoc.Spec.Elasticsearch.NamespacedName())
	userLabels[AssociationLabelName] = assoc.Name
	usrKey := userKey(assoc)
	expectedUser := &estype.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      usrKey.Name,
			Namespace: usrKey.Namespace,
			Labels:    userLabels,
		},
		Spec: estype.UserSpec{
			Name:         InternalKibanaServerUserName,
			PasswordHash: string(bcryptHash),
			UserRoles:    []string{user.KibanaUserBuiltinRole},
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
			// TODO compare labels
			return !reflect.DeepEqual(expectedUser.Spec, reconciledUser.Spec)
		},
		UpdateReconciled: func() {
			reconciledUser.Spec = expectedUser.Spec
		},
	})

}
