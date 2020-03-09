// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"bytes"
	"context"

	"go.elastic.co/apm"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// elasticsearchUserName identifies the associated user in Elasticsearch namespace.
func elasticsearchUserName(associated commonv1.Associated, userSuffix string) string {
	// must be namespace-aware since we might have several associated instances running in
	// different namespaces with the same name: we need one user for each
	// in the Elasticsearch namespace
	return associated.GetNamespace() + "-" + associated.GetName() + "-" + userSuffix
}

// userSecretObjectName identifies the associated secret object.
func userSecretObjectName(associated commonv1.Associated, userSuffix string) string {
	// does not need to be namespace aware, since it lives in associated object namespace.
	return associated.GetName() + "-" + userSuffix
}

// UserKey is the namespaced name to identify the user resource created by the controller.
func UserKey(associated commonv1.Associated, userSuffix string) types.NamespacedName {
	esNamespace := associated.ElasticsearchRef().Namespace
	if esNamespace == "" {
		// no namespace given, default to the associated object's one
		esNamespace = associated.GetNamespace()
	}
	return types.NamespacedName{
		// user lives in the ES namespace
		Namespace: esNamespace,
		Name:      elasticsearchUserName(associated, userSuffix),
	}
}

// secretKey is the namespaced name to identify the secret containing the password for the user.
// It uses the same resource name as the associated user.
func secretKey(associated commonv1.Associated, userSuffix string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: associated.GetNamespace(),
		Name:      userSecretObjectName(associated, userSuffix),
	}
}

// ClearTextSecretKeySelector creates a SecretKeySelector for the associated user secret
func ClearTextSecretKeySelector(associated commonv1.Associated, userSuffix string) *corev1.SecretKeySelector {
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: userSecretObjectName(associated, userSuffix),
		},
		Key: elasticsearchUserName(associated, userSuffix),
	}
}

// ReconcileEsUser creates a User resource and a corresponding secret or updates those as appropriate.
func ReconcileEsUser(
	ctx context.Context,
	c k8s.Client,
	s *runtime.Scheme,
	associated commonv1.Associated,
	labels map[string]string,
	userRoles string,
	userObjectSuffix string,
	es esv1.Elasticsearch,
) error {
	span, _ := apm.StartSpan(ctx, "reconcile_es_user", tracing.SpanTypeApp)
	defer span.End()

	pw := common.RandomPasswordBytes()

	secKey := secretKey(associated, userObjectSuffix)
	usrKey := UserKey(associated, userObjectSuffix)
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secKey.Name,
			Namespace: secKey.Namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			usrKey.Name: pw,
		},
	}

	reconciledSecret := corev1.Secret{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      associated,
		Expected:   &expectedSecret,
		Reconciled: &reconciledSecret,
		NeedsUpdate: func() bool {
			_, ok := reconciledSecret.Data[usrKey.Name]
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

	reconciledPw := reconciledSecret.Data[usrKey.Name] // make sure we don't constantly update the password
	bcryptHash, err := bcrypt.GenerateFromPassword(reconciledPw, bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// analogous to the associated secret: a user Secret goes on the Elasticsearch side of the association
	// we apply the ES cluster labels ("user belongs to that ES cluster")
	// and the association label ("for that associated object")

	// merge the association labels provided by the controller with the one needed for a user
	userLabels := esuser.AssociatedUserLabels(es)
	for key, value := range labels {
		userLabels[key] = value
	}

	expectedEsUser := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      usrKey.Name,
			Namespace: usrKey.Namespace,
			Labels:    userLabels,
		},
		Data: map[string][]byte{
			esuser.UserNameField:     []byte(usrKey.Name),
			esuser.PasswordHashField: bcryptHash,
			esuser.UserRolesField:    []byte(userRoles),
		},
	}

	reconciledEsSecret := corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &es, // user is owned by the ES resource
		Expected:   expectedEsUser,
		Reconciled: &reconciledEsSecret,
		NeedsUpdate: func() bool {
			return !hasExpectedLabels(expectedEsUser, &reconciledEsSecret) ||
				!bytes.Equal(expectedEsUser.Data[esuser.UserNameField], reconciledEsSecret.Data[esuser.UserNameField]) ||
				!bytes.Equal(expectedEsUser.Data[esuser.UserRolesField], reconciledEsSecret.Data[esuser.UserRolesField]) ||
				bcrypt.CompareHashAndPassword(reconciledEsSecret.Data[esuser.PasswordHashField], reconciledPw) != nil
		},
		UpdateReconciled: func() {
			setExpectedLabels(expectedEsUser, &reconciledEsSecret)
			reconciledEsSecret.Data = expectedEsUser.Data
		},
	})
}

// hasExpectedLabels does a left-biased comparison ensuring all key/value pairs in expected exist in actual.
func hasExpectedLabels(expected, actual metav1.Object) bool {
	actualLabels := actual.GetLabels()
	for k, v := range expected.GetLabels() {
		if actualLabels[k] != v {
			return false
		}
	}
	return true
}

// setExpectedLabels set the labels from expected into actual.
func setExpectedLabels(expected, actual metav1.Object) {
	actualLabels := actual.GetLabels()
	if actualLabels == nil {
		actualLabels = make(map[string]string)
	}
	for k, v := range expected.GetLabels() {
		actualLabels[k] = v
	}
	actual.SetLabels(actualLabels)
}
