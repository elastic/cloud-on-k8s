// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
		customUsers  corev1.SecretList
		defaultUsers []ClearTextCredentials
	}
	tests := []struct {
		name       string
		args       args
		wantErr    bool
		assertions func([]user.User)
	}{
		{
			name:    "default case: one kibana user plus internal + external users",
			wantErr: false,
			args: args{
				customUsers: corev1.SecretList{
					Items: []corev1.Secret{
						{
							ObjectMeta: v1.ObjectMeta{
								Namespace: "default",
								Name:      "kibana-user",
							},
							Data: map[string][]byte{
								user.UserName:     []byte("kibana-user"),
								user.PasswordHash: []byte("skfjdkf"),
								user.UserRoles:    []byte(KibanaSystemUserBuiltinRole),
							},
						},
					},
				},
				defaultUsers: []ClearTextCredentials{
					*internalUsers,
					*externalUsers,
				},
			},
			assertions: func(users []user.User) {
				assert.Equal(t, len(users), 5)
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
			name:    "multiple custom users",
			wantErr: false,
			args: args{
				customUsers: corev1.SecretList{
					Items: []corev1.Secret{
						{
							ObjectMeta: v1.ObjectMeta{
								Namespace: "default",
								Name:      "kibana-user",
							},
							Data: map[string][]byte{
								user.UserName:     []byte("kibana-user"),
								user.PasswordHash: []byte("skfjdkf"),
								user.UserRoles:    []byte("KibanaSystemUserBuiltinRole"),
							},
						},
						{
							ObjectMeta: v1.ObjectMeta{
								Namespace: "default",
								Name:      "foo-user",
							},
							Data: map[string][]byte{
								user.UserName:     []byte("foo-user"),
								user.PasswordHash: []byte("alskdfjslkdfjlsk"),
								user.UserRoles:    []byte("kibana-user"),
							},
						},
					},
				},
				defaultUsers: nil,
			},
			assertions: func(users []user.User) {
				assert.Equal(t, len(users), 2)
				containsAllNames(t, []string{"kibana-user", "foo-user"}, users)
			},
		},
		{
			name:    "invalid custom users raise an error",
			wantErr: true,
			args: args{
				customUsers: corev1.SecretList{
					Items: []corev1.Secret{
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
			assertions: func(users []user.User) {
				assert.Equal(t, len(users), 0)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := aggregateAllUsers(tt.args.customUsers, tt.args.defaultUsers...)
			if (err != nil) != tt.wantErr {
				t.Errorf("aggregateAllUsers(...) error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.assertions != nil {
				tt.assertions(got)
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
			t.Errorf("Expected %s but was not contained in %v", n, dict)
		}
	}
}
