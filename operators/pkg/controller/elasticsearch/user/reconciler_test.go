// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/user"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/magiconair/properties/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_aggregateAllUsers(t *testing.T) {
	nsn := types.NamespacedName{
		Namespace: "default",
		Name:      "foo",
	}
	internalUsers := NewInternalUserCredentials(nsn)
	externalUsers := NewExternalUserCredentials(nsn)
	type args struct {
		customUsers  v1alpha1.UserList
		defaultUsers []ClearTextCredentials
	}
	tests := []struct {
		name       string
		args       args
		assertions func([]user.User, []func(client k8s.Client) error)
	}{
		{
			name: "default case: one kibana user plus internal + external users",
			args: args{
				customUsers: v1alpha1.UserList{
					Items: []v1alpha1.User{
						{
							ObjectMeta: v1.ObjectMeta{
								Namespace: "default",
								Name:      "kibana-user",
							},
							Spec: v1alpha1.UserSpec{
								Name:         "kibana-user",
								PasswordHash: "skfjdkf",
								UserRoles:    []string{KibanaSystemUserBuiltinRole},
							},
						},
					},
				},
				defaultUsers: []ClearTextCredentials{
					*internalUsers,
					*externalUsers,
				},
			},
			assertions: func(users []user.User, status []func(client k8s.Client) error) {
				assert.Equal(t, len(users), 5)
				assert.Equal(t, len(status), 1)
				containsAllNames(
					t,
					[]string{
						"kibana-user",
						ExternalUserName,
						InternalControllerUserName, InternalProbeUserName, InternalReloadCredsUserName,
					},
					users,
				)
			},
		},
		{
			name: "multiple custom users",
			args: args{
				customUsers: v1alpha1.UserList{
					Items: []v1alpha1.User{
						{
							ObjectMeta: v1.ObjectMeta{
								Namespace: "default",
								Name:      "kibana-user",
							},
							Spec: v1alpha1.UserSpec{
								Name:         "kibana-user",
								PasswordHash: "skfjdkf",
								UserRoles:    []string{KibanaSystemUserBuiltinRole},
							},
						},
						{
							ObjectMeta: v1.ObjectMeta{
								Namespace: "default",
								Name:      "foo-user",
							},
							Spec: v1alpha1.UserSpec{
								Name:         "foo-user",
								PasswordHash: "alskdfjslkdfjlsk",
								UserRoles:    []string{"kibana-user"},
							},
						},
					},
				},
				defaultUsers: nil,
			},
			assertions: func(users []user.User, status []func(client k8s.Client) error) {
				assert.Equal(t, len(users), 2)
				assert.Equal(t, len(status), 2)
				containsAllNames(t, []string{"kibana-user", "foo-user"}, users)
			},
		},
		{
			name: "invalid custom users are ignored",
			args: args{
				customUsers: v1alpha1.UserList{
					Items: []v1alpha1.User{
						{
							ObjectMeta: v1.ObjectMeta{
								Namespace: "default",
								Name:      "invalid-user",
							},
						},
					},
				},
				defaultUsers: []ClearTextCredentials{
					*externalUsers,
				},
			},
			assertions: func(users []user.User, status []func(client k8s.Client) error) {
				assert.Equal(t, len(users), 1)
				assert.Equal(t, len(status), 1)
				containsAllNames(t, []string{ExternalUserName}, users)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := aggregateAllUsers(tt.args.customUsers, tt.args.defaultUsers...)
			if tt.assertions != nil {
				tt.assertions(got, got1)
			}
		})
	}
}

func containsAllNames(t *testing.T, names []string, users []user.User) {
	dict := make(map[string]struct{})
	for _, u := range users {
		dict[u.Id()] = struct{}{}
	}
	for _, n := range names {
		_, ok := dict[n]
		if !ok {
			t.Errorf("Epected %s but was not contained in %v", n, dict)
		}
	}
}
