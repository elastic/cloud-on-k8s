// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"reflect"

	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	kbctl "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

const (
	// kibanaUser is the name of the Kibana server Elasticsearch user.
	// Also used to suffix user and secret resources.
	kibanaUser = "kibana-user"
)

// KibanaUserObjectName identifies the Kibana user object (secret/user CRD).
func KibanaUserObjectName(kibana types.NamespacedName) string {
	// must be namespace-aware since we might have several kibanas running in
	// different namespaces with the same name: we need one user for each
	// in the Elasticsearch namespace
	return kibana.Namespace + "-" + kibana.Name + "-" + kibanaUser
}

// KibanaUserKey is the namespaced name to identify the user resource created by the controller.
func KibanaUserKey(kibana kbtype.Kibana, esNamespace string) types.NamespacedName {
	if esNamespace == "" {
		// no namespace given, default to Kibana's one
		esNamespace = kibana.Namespace
	}
	return types.NamespacedName{
		// user lives in the ES namespace
		Namespace: esNamespace,
		Name:      KibanaUserObjectName(k8s.ExtractNamespacedName(&kibana)),
	}
}

// KibanaUserSecretObjectName identifies the Kibana secret object.
func KibanaUserSecretObjectName(kibana types.NamespacedName) string {
	// does not need to be namespace aware, since it lives in Kibana namespace.
	return kibana.Name + "-" + kibanaUser
}

// KibanaUserSecret is the namespaced name to identify the secret containing the password for the Kibana user.
// It uses the same resource name as the Kibana user.
func KibanaUserSecretKey(kibana types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Namespace: kibana.Namespace,
		Name:      KibanaUserSecretObjectName(kibana),
	}
}

// KibanaUserSelector creates a SecretKeySelector for the Kibana user secret
func KibanaUserSecretSelector(kibana kbtype.Kibana) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: KibanaUserSecretObjectName(k8s.ExtractNamespacedName(&kibana)),
		},
		Key: kibanaUser,
	}
}

// reconcileEsUser creates a User resource and a corresponding secret or updates those as appropriate.
func reconcileEsUser(c k8s.Client, s *runtime.Scheme, kibana kbtype.Kibana, es types.NamespacedName) error {
	// TODO: more flexible user-name (suffixed-trimmed?) so multiple associations do not conflict
	pw := common.RandomPasswordBytes()
	// the secret will be on the Kibana side of the association so we are applying the Kibana labels here
	secretLabels := kbctl.NewLabels(kibana.Name)
	secretLabels[AssociationLabelName] = kibana.Name
	secKey := KibanaUserSecretKey(k8s.ExtractNamespacedName(&kibana))
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secKey.Name,
			Namespace: secKey.Namespace,
			Labels:    secretLabels,
		},
		Data: map[string][]byte{
			kibanaUser: pw,
		},
	}

	reconciledSecret := corev1.Secret{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &kibana,
		Expected:   &expectedSecret,
		Reconciled: &reconciledSecret,
		NeedsUpdate: func() bool {
			_, ok := reconciledSecret.Data[kibanaUser]
			return !ok || !hasExpectedLabels(&expectedSecret, &reconciledSecret)
		},
		UpdateReconciled: func() {
			setExpectedLabels(&expectedSecret, &reconciledSecret)
			reconciledSecret.Data = expectedSecret.Data
		},
	})
	if err != nil {
		return err
	}
	reconciledPw := reconciledSecret.Data[kibanaUser] // make sure we don't constantly update the password

	bcryptHash, err := bcrypt.GenerateFromPassword(reconciledPw, bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// analogous to the secret: the user goes on the Elasticsearch side of the association
	// we apply the ES cluster labels ("user belongs to that ES cluster")
	// and the association label ("for that Kibana association")
	userLabels := label.NewLabels(es)
	userLabels[AssociationLabelName] = kibana.Name
	usrKey := KibanaUserKey(kibana, es.Namespace)
	expectedUser := &estype.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      usrKey.Name,
			Namespace: usrKey.Namespace,
			Labels:    userLabels,
		},
		Spec: estype.UserSpec{
			Name:         kibanaUser,
			PasswordHash: string(bcryptHash),
			UserRoles:    []string{user.KibanaSystemUserBuiltinRole},
		},
		Status: estype.UserStatus{
			Phase: estype.UserPending,
		},
	}

	reconciledUser := estype.User{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &kibana, // user is owned by the Kibana resource
		Expected:   expectedUser,
		Reconciled: &reconciledUser,
		NeedsUpdate: func() bool {
			return !hasExpectedLabels(expectedUser, &reconciledUser) ||
				expectedUser.Spec.Name != reconciledUser.Spec.Name ||
				!reflect.DeepEqual(expectedUser.Spec.UserRoles, reconciledUser.Spec.UserRoles) ||
				bcrypt.CompareHashAndPassword([]byte(reconciledUser.Spec.PasswordHash), reconciledPw) != nil
		},
		UpdateReconciled: func() {
			setExpectedLabels(expectedUser, &reconciledUser)
			reconciledUser.Spec = expectedUser.Spec
		},
	})

}
