// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	eslabel "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	associated commonv1.Associated,
	labels map[string]string,
	userRoles string,
	userObjectSuffix string,
	es esv1.Elasticsearch,
) error {
	span, _ := apm.StartSpan(ctx, "reconcile_es_user", tracing.SpanTypeApp)
	defer span.End()

	// Add the Elasticsearch name, this is only intended to help the user to filter on these resources
	labels[eslabel.ClusterNameLabelName] = es.Name

	secKey := secretKey(associated, userObjectSuffix)
	usrKey := UserKey(associated, userObjectSuffix)
	expectedSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secKey.Name,
			Namespace: secKey.Namespace,
			Labels:    common.AddCredentialsLabel(labels),
		},
		Data: map[string][]byte{},
	}

	var password []byte
	// reuse the existing password if there's one
	var existingSecret corev1.Secret
	err := c.Get(k8s.ExtractNamespacedName(&expectedSecret), &existingSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if existingPassword, exists := existingSecret.Data[usrKey.Name]; exists {
		password = existingPassword
	} else {
		password = common.FixedLengthRandomPasswordBytes()
	}
	expectedSecret.Data[usrKey.Name] = password

	if _, err := reconciler.ReconcileSecret(c, expectedSecret, associated); err != nil {
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

	expectedEsUser := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      usrKey.Name,
			Namespace: usrKey.Namespace,
			Labels:    userLabels,
		},
		Data: map[string][]byte{
			esuser.UserNameField:  []byte(usrKey.Name),
			esuser.UserRolesField: []byte(userRoles),
		},
	}

	// reuse the existing hash if valid
	bcryptHash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	var existingUserSecret corev1.Secret
	if err := c.Get(k8s.ExtractNamespacedName(&expectedEsUser), &existingUserSecret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if existingHash, exists := existingUserSecret.Data[esuser.PasswordHashField]; exists {
		if bcrypt.CompareHashAndPassword(existingHash, password) == nil {
			bcryptHash = existingHash
		}
	}
	expectedEsUser.Data[esuser.PasswordHashField] = bcryptHash

	owner := es // user is owned by the es resource in es namespace
	_, err = reconciler.ReconcileSecret(c, expectedEsUser, &owner)
	return err
}
