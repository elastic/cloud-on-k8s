// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func Test_retrieveAssociatedUsers(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "ns"},
	}
	tests := []struct {
		name    string
		secrets []runtime.Object
		want    users
	}{
		{
			name:    "no associated user secret",
			secrets: nil,
			want:    users{},
		},
		{
			name: "some associated users secrets",
			secrets: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: es.Namespace,
						Name:      "user1",
						Labels:    AssociatedUserLabels(es),
					},
					Data: map[string][]byte{
						UserNameField:     []byte("user1"),
						PasswordHashField: []byte("passwordHash1"),
						UserRolesField:    []byte("role1,role2"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: es.Namespace,
						Name:      "user2",
						Labels:    AssociatedUserLabels(es),
					},
					Data: map[string][]byte{
						UserNameField:     []byte("user2"),
						PasswordHashField: []byte("passwordHash2"),
						UserRolesField:    []byte("role1,role2,role3"),
					},
				},
			},
			want: users{
				{Name: "user1", PasswordHash: []byte("passwordHash1"), Roles: []string{"role1", "role2"}},
				{Name: "user2", PasswordHash: []byte("passwordHash2"), Roles: []string{"role1", "role2", "role3"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.secrets...)
			users, err := retrieveAssociatedUsers(c, es)
			require.NoError(t, err)
			require.Equal(t, tt.want, users)
		})
	}
}

func Test_parseAssociatedUserSecret(t *testing.T) {
	type args struct {
		secret v1.Secret
	}
	tests := []struct {
		name    string
		args    args
		want    AssociatedUser
		wantErr bool
	}{
		{
			name:    "Simple kibana example",
			wantErr: false,
			args: args{
				secret: v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ns2-kibana-sample-kibana-user",
						Namespace: "default",
					},
					Data: map[string][]byte{
						UserNameField:     []byte("ns2-kibana-sample-kibana-user"),
						PasswordHashField: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
						UserRolesField:    []byte("kibana_system"),
					},
				},
			},
			want: AssociatedUser{
				Name:         "ns2-kibana-sample-kibana-user",
				PasswordHash: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
				Roles:        []string{"kibana_system"},
			},
		},
		{
			name:    "Multi-roles example",
			wantErr: false,
			args: args{
				secret: v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ns2-kibana-sample-kibana-user",
						Namespace: "default",
					},
					Data: map[string][]byte{
						UserNameField:     []byte("ns2-kibana-sample-kibana-user"),
						PasswordHashField: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
						UserRolesField:    []byte("kibana_system1,kibana_system2,kibana_system3"),
					},
				},
			},
			want: AssociatedUser{
				Name:         "ns2-kibana-sample-kibana-user",
				PasswordHash: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
				Roles:        []string{"kibana_system1", "kibana_system2", "kibana_system3"},
			},
		},
		{
			name:    "User name is missing",
			wantErr: true,
			args: args{
				secret: v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ns2-kibana-sample-kibana-user",
						Namespace: "default",
					},
					Data: map[string][]byte{
						PasswordHashField: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
						UserRolesField:    []byte("kibana_system"),
					},
				},
			},
			want: AssociatedUser{},
		},
		{
			name:    "Password is missing",
			wantErr: true,
			args: args{
				secret: v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ns2-kibana-sample-kibana-user",
						Namespace: "default",
					},
					Data: map[string][]byte{
						UserNameField:  []byte("ns2-kibana-sample-kibana-user"),
						UserRolesField: []byte("kibana_system"),
					},
				},
			},
			want: AssociatedUser{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAssociatedUserSecret(tt.args.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAssociatedUserSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAssociatedUserSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
