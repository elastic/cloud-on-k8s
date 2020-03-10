// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultUserName = "enterprise_search"
)

func DefaultUserRef(ents entsv1beta1.EnterpriseSearch) types.NamespacedName {
	return types.NamespacedName{
		Namespace: ents.Namespace,
		Name:      name.DefaultUser(ents.Name),
	}
}

type User struct {
	Name     string
	Password []byte
}

func (u User) Secret(ents entsv1beta1.EnterpriseSearch) corev1.Secret {
	nsn := DefaultUserRef(ents)
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsn.Namespace,
			Name:      nsn.Name,
			Labels:    NewLabels(ents.Name),
		},
		Data: map[string][]byte{
			u.Name: u.Password,
		},
	}
}

// TODO: should this be a config setting instead?
// TODO: this should not be printed out on stdout, but is :(
func DefaultUserEnvVar(ents entsv1beta1.EnterpriseSearch) corev1.EnvVar {
	return corev1.EnvVar{
		Name: "ENT_SEARCH_DEFAULT_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: DefaultUserRef(ents).Name,
				},
				Key: DefaultUserName,
			},
		},
	}
}

func GenerateDefaultUser() User {
	return User{
		Name:     DefaultUserName,
		Password: user.RandomPasswordBytes(),
	}
}

func ReconcileDefaultUser(c k8s.Client, ents entsv1beta1.EnterpriseSearch, scheme *runtime.Scheme) error {
	user := GenerateDefaultUser()
	expected := user.Secret(ents)
	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &ents,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			if !maps.IsSubset(expected.Labels, reconciled.Labels) {
				// reconciled does not have expected labels
				return true
			}
			password, ok := reconciled.Data[DefaultUserName]
			if !ok || len(password) == 0 {
				// no password set
				return true
			}
			// otherwise, keep the existing password
			return false
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data
		},
	})
}
