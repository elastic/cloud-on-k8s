// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/user"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"

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

	var allUsers []user.User
	var customUsers v1alpha1.UserList
	if err := c.List(&client.ListOptions{
		LabelSelector: label.NewLabelSelectorForElasticsearch(es),
		Namespace:     es.Namespace,
	}, &customUsers); err != nil {
		return nil, err
	}

	var statusUpdates []func() error
	for _, u := range customUsers.Items {
		// do minimal sanity checking on externally created users
		if u.IsEmpty() {
			log.Info("Ignoring invalid", "user", u)
			statusUpdates = append(statusUpdates, phaseUpdate(c, u, v1alpha1.UserInvalid))
			continue
		}
		statusUpdates = append(statusUpdates, phaseUpdate(c, u, v1alpha1.UserPropagated))
		allUsers = append(allUsers, &u)
	}

	for _, u := range users {
		allUsers = append(allUsers, user.User(u))
	}
	elasticUsersRolesSecret, err := NewElasticUsersCredentialsAndRoles(nsn, allUsers, roles)
	if err != nil {
		return nil, err
	}
	if err := ReconcileUserCredentialsSecret(c, scheme, es, elasticUsersRolesSecret); err != nil {
		return nil, err
	}

	// We are delaying  user status updates to happen only after the reconciliation went through.
	// This has the slight disadvantage that user status updates don't happen on early returns but the reduced complexity
	// of avoiding defers and named returns makes it worthwhile given the user status is of limited use anyway.
	return &internalUsers, applyDelayedUpdates(statusUpdates)
}

func phaseUpdate(c k8s.Client, user v1alpha1.User, phase v1alpha1.UserPhase) func() error {
	user.Status.Phase = phase
	return func() error {
		if err := c.Status().Update(&user); err != nil {
			return errors.Wrapf(err, "Failed to update status for user %v", k8s.ExtractNamespacedName(&user))
		}
		return nil
	}
}

func applyDelayedUpdates(updates []func() error) error {
	var errs []error
	for _, f := range updates {
		if err := f(); err != nil {
			errs = append(errs, err)
		}
	}
	return k8serrors.NewAggregate(errs)
}
