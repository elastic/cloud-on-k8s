// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"bytes"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// InternalPamServerUserName is a user to be used by the Apm server when interacting with ES.
	InternalApmServerUserName = "elastic-internal-apm"
	apmUser                   = "apm-user"
)

// name to identify the Apm user object (secret/user CRD)
func apmUserObjectName(assocName string) string {
	return assocName + "-" + InternalApmServerUserName
}

// userKey is the namespaced name to identify the customer user resource created by the controller.
func userKey(apm apmtype.ApmServer) *types.NamespacedName {

	ref := apm.Spec.Output.Elasticsearch.ElasticsearchRef
	if ref == nil {
		return nil
	}
	return &types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      userName(apm),
	}
}

func userName(apm apmtype.ApmServer) string {
	return apm.Namespace + "-" + apm.Name + "-" + apmUser
}

// secretKey is the namespaced name to identify the secret containing the password for the Apm user.
func secretKey(apm apmtype.ApmServer) types.NamespacedName {
	return types.NamespacedName{
		Namespace: apm.Namespace,
		Name:      apmUserObjectName(apm.Name),
	}
}

// creates a SecretKeySelector selecting the Apm user secret for the given association
func clearTextSecretKeySelector(apm apmtype.ApmServer) *corev1.SecretKeySelector {
	usrKey := userKey(apm)
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: apmUserObjectName(apm.Name),
		},
		Key: usrKey.Name,
	}
}

// reconcileEsUser creates a User resource and a corresponding secret or updates those as appropriate.
func reconcileEsUser(c k8s.Client, s *runtime.Scheme, apm apmtype.ApmServer) error {
	// TODO: more flexible user-name (suffixed-trimmed?) so multiple associations do not conflict
	pw := common.RandomPasswordBytes()
	// the secret will be on the Apm side of the association so we are applying the Apm labels here
	secretLabels := apmserver.NewLabels(apm.Name)
	secretLabels[AssociationLabelName] = apm.Name
	// add ES labels
	for k, v := range label.NewLabels(apm.Spec.Output.Elasticsearch.ElasticsearchRef.NamespacedName()) {
		secretLabels[k] = v
	}
	secKey := secretKey(apm)
	usrKey := userKey(apm)
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secKey.Name,
			Namespace: secKey.Namespace,
			Labels:    secretLabels,
		},
		Data: map[string][]byte{
			usrKey.Name: pw,
		},
	}

	reconciledSecret := corev1.Secret{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &apm,
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
	expectedSecret.Data = reconciledSecret.Data // make sure we don't constantly update the password

	reconciledPw := expectedSecret.Data[usrKey.Name]
	bcryptHash, err := bcrypt.GenerateFromPassword(reconciledPw, bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// analogous to the secret: the user goes on the Elasticsearch side of the association, we apply the ES labels for visibility
	userLabels := common.NewLabels(apm.Spec.Output.Elasticsearch.ElasticsearchRef.NamespacedName())
	userLabels[AssociationLabelName] = apm.Name
	expectedESUser := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      usrKey.Name,
			Namespace: usrKey.Namespace,
			Labels:    userLabels,
		},
		Data: map[string][]byte{
			common.UserName:     []byte(usrKey.Name),
			common.PasswordHash: bcryptHash,
			// TODO: lower privileges, but requires specifying a custom role
			common.UserRoles: []byte("superuser"),
		},
	}

	reconciledEsSecret := corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     s,
		Owner:      &apm,
		Expected:   expectedESUser,
		Reconciled: &reconciledEsSecret,
		NeedsUpdate: func() bool {
			return !hasExpectedLabels(expectedESUser, &reconciledSecret) ||
				!bytes.Equal(expectedESUser.Data["Name"], reconciledEsSecret.Data["Name"]) ||
				!bytes.Equal(expectedESUser.Data["UserRoles"], reconciledEsSecret.Data["UserRoles"]) ||
				bcrypt.CompareHashAndPassword(reconciledPw, []byte(reconciledEsSecret.Data["PasswordHash"])) == nil
		},
		UpdateReconciled: func() {
			setExpectedLabels(expectedESUser, &reconciledEsSecret)
			reconciledEsSecret.Data = expectedESUser.Data
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
