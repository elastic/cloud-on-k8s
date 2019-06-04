// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"bytes"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	commonuser "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	kblabel "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// kibanaUser is the name of the Kibana server Elasticsearch user.
	// Also used to suffix user and secret resources.
	kibanaUser = "kibana-user"
)

// KibanaUserName identifies the Kibana user.
func KibanaUserName(kibana types.NamespacedName) string {
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
		Name:      KibanaUserName(k8s.ExtractNamespacedName(&kibana)),
	}
}

// KibanaUserSecretObjectName identifies the Kibana secret object.
func KibanaUserSecretObjectName(kibana types.NamespacedName) string {
	// does not need to be namespace aware, since it lives in Kibana namespace.
	return kibana.Name + "-" + kibanaUser
}

// KibanaUserSecretKey is the namespaced name to identify the secret containing the password for the Kibana user.
// It uses the same resource name as the Kibana user.
func KibanaUserSecretKey(kibana types.NamespacedName) types.NamespacedName {
	return types.NamespacedName{
		Namespace: kibana.Namespace,
		Name:      KibanaUserSecretObjectName(kibana),
	}
}

// KibanaUserSecretSelector creates a SecretKeySelector for the Kibana user secret
func KibanaUserSecretSelector(kibana kbtype.Kibana) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: KibanaUserSecretObjectName(k8s.ExtractNamespacedName(&kibana)),
		},
		Key: KibanaUserName(k8s.ExtractNamespacedName(&kibana)),
	}
}

// reconcileEsUser creates a User resource and a corresponding secret or updates those as appropriate.
func reconcileEsUser(c k8s.Client, s *runtime.Scheme, kibana kbtype.Kibana, es types.NamespacedName) error {
	// the secret will be on the Kibana side of the association so we are applying the Kibana labels here
	secretLabels := kblabel.NewLabels(kibana.Name)
	secretLabels[AssociationLabelName] = kibana.Name
	secKey := KibanaUserSecretKey(k8s.ExtractNamespacedName(&kibana))
	pw := commonuser.RandomPasswordBytes()
	username := KibanaUserName(k8s.ExtractNamespacedName(&kibana))
	expectedKibanaSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secKey.Name,
			Namespace: secKey.Namespace,
			Labels:    secretLabels,
		},
		Data: map[string][]byte{
			username: pw,
		},
	}

	reconciledSecret := corev1.Secret{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &kibana,
		Expected:   &expectedKibanaSecret,
		Reconciled: &reconciledSecret,
		NeedsUpdate: func() bool {
			_, ok := reconciledSecret.Data[username]
			return !ok || !hasExpectedLabels(&expectedKibanaSecret, &reconciledSecret)
		},
		UpdateReconciled: func() {
			setExpectedLabels(&expectedKibanaSecret, &reconciledSecret)
			reconciledSecret.Data = expectedKibanaSecret.Data
		},
	})
	if err != nil {
		return err
	}

	reconciledPw := reconciledSecret.Data[username] // make sure we don't constantly update the password
	bcryptHash, err := bcrypt.GenerateFromPassword(reconciledPw, bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// analogous to the Kibana secret: a user Secret goes on the Elasticsearch side of the association
	// we apply the ES cluster labels ("user belongs to that ES cluster")
	// and the association label ("for that Kibana association")
	userLabels := commonuser.NewLabels(es)
	userLabels[AssociationLabelName] = kibana.Name
	usrKey := KibanaUserKey(kibana, es.Namespace)

	expectedEsUser := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      usrKey.Name,
			Namespace: usrKey.Namespace,
			Labels:    userLabels,
		},
		Data: map[string][]byte{
			commonuser.UserName:     []byte(username),
			commonuser.PasswordHash: bcryptHash,
			commonuser.UserRoles:    []byte(user.KibanaSystemUserBuiltinRole),
		},
	}

	reconciledEsSecret := corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &kibana, // user is owned by the Kibana resource
		Expected:   expectedEsUser,
		Reconciled: &reconciledEsSecret,
		NeedsUpdate: func() bool {
			return !hasExpectedLabels(expectedEsUser, &reconciledEsSecret) ||
				!bytes.Equal(expectedEsUser.Data["Name"], reconciledEsSecret.Data["Name"]) ||
				!bytes.Equal(expectedEsUser.Data["UserRoles"], reconciledEsSecret.Data["UserRoles"]) ||
				bcrypt.CompareHashAndPassword(reconciledPw, []byte(reconciledEsSecret.Data["PasswordHash"])) == nil
		},
		UpdateReconciled: func() {
			setExpectedLabels(expectedEsUser, &reconciledEsSecret)
			reconciledEsSecret.Data = expectedEsUser.Data
		},
	})
}
