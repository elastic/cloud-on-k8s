// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileUserCredentialsSecret creates or updates the given credentials.
func ReconcileUserCredentialsSecret(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	creds UserCredentials,
) error {
	expected := creds.Secret()
	reconciled := &corev1.Secret{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			return creds.NeedsUpdate(*reconciled)
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data // only update data, keep the rest
		},
	})
	if err == nil {
		// expected creds have been updated to reflect the state on the API server
		creds.Reset(*reconciled)
	}
	return err
}

func aggregateAllUsers(customUsers corev1.SecretList, defaultUsers ...ClearTextCredentials) ([]user.User, error) {
	var allUsers []user.User
	for _, clearText := range defaultUsers {
		for _, u := range clearText.Users() {
			usr := u
			allUsers = append(allUsers, usr)
		}
	}

	for _, s := range customUsers.Items {
		usr, err := user.NewExternalUserFromSecret(s)
		if err != nil {
			return nil, err
		}
		allUsers = append(allUsers, &usr)
	}
	return allUsers, nil
}

// ReconcileUsers aggregates two clear-text secrets into an ES readable secret.
// The 'internal-users' secret contains credentials for use by other stack components like
// Kibana and for use by the controller or liveliness probes.
// The 'elastic-user' secret contains credentials for the reserved bootstrap user 'elastic'
// which needs to be known by users in order to be able to interact with the cluster.
// The aggregated secret is used to mount a 'users' file consisting of a sequence of username:bcrypt hashes
// into the Elasticsearch config directory which the file realm of ES security can directly understand.
// A second file called 'users_roles' is contained in this third secret as well which describes
// role assignments for the users specified in the first file.
func ReconcileUsers(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
) (*InternalUsers, error) {

	nsn := k8s.ExtractNamespacedName(&es)
	internalSecrets := NewInternalUserCredentials(nsn)
	if err := ReconcileUserCredentialsSecret(c, scheme, es, internalSecrets); err != nil {
		return nil, err
	}

	externalSecrets := NewExternalUserCredentials(nsn)
	if err := ReconcileUserCredentialsSecret(c, scheme, es, externalSecrets); err != nil {
		return nil, err
	}

	var customUsers corev1.SecretList
	// TODO sabo fix
	// if err := c.List(&client.ListOptions{
	// 	LabelSelector: user.NewLabelSelectorForElasticsearch(es),
	// 	Namespace:     es.Namespace,
	// }, &customUsers); err != nil {
	// 	return nil, err
	// }
	ns := client.InNamespace(es.Namespace)
	if err := c.List(&customUsers, ns); err != nil {
		return nil, err
	}

	allUsers, err := aggregateAllUsers(customUsers, *internalSecrets, *externalSecrets)
	if err != nil {
		return nil, err
	}
	elasticUsersRolesSecret, err := NewElasticUsersCredentialsAndRoles(nsn, allUsers, PredefinedRoles)
	if err != nil {
		return nil, err
	}
	if err := ReconcileUserCredentialsSecret(c, scheme, es, elasticUsersRolesSecret); err != nil {
		return nil, err
	}

	return NewInternalUsersFrom(*internalSecrets), nil
}
