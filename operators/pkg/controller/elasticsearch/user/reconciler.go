// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/user"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("secret")
)

// ReconcileSecret creates or updates the given credentials.
func ReconcileUserCredentialsSecret(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
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
	es v1alpha1.ElasticsearchCluster,
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

	users := internalSecrets.Users()
	internalUsers := NewInternalUsersFrom(users)
	users = append(users, externalSecrets.Users()...)
	roles := PredefinedRoles

	var commonUsers []user.User
	var customUsers v1alpha1.UserList
	if err := c.List(&client.ListOptions{
		LabelSelector: label.NewLabelSelectorForElasticsearch(es),
	}, &customUsers); err != nil {
		return nil, err
	}
	for _, u := range customUsers.Items {
		commonUsers = append(commonUsers, &u)
	}

	for _, u := range users {
		commonUsers = append(commonUsers, user.User(u))
	}
	elasticUsersRolesSecret, err := NewElasticUsersCredentialsAndRoles(nsn, commonUsers, roles)
	if err != nil {
		return nil, err
	}
	if err := ReconcileUserCredentialsSecret(c, scheme, es, elasticUsersRolesSecret); err != nil {
		return nil, err
	}

	return &internalUsers, err
}
