// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewProvidedUserFromSecret(t *testing.T) {
	type args struct {
		secret v1.Secret
	}
	tests := []struct {
		name    string
		args    args
		want    ProvidedUser
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
						UserName:     []byte("ns2-kibana-sample-kibana-user"),
						PasswordHash: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
						UserRoles:    []byte("kibana_system"),
					},
				},
			},
			want: ProvidedUser{
				name:     "ns2-kibana-sample-kibana-user",
				password: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
				roles:    []string{"kibana_system"},
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
						UserName:     []byte("ns2-kibana-sample-kibana-user"),
						PasswordHash: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
						UserRoles:    []byte("kibana_system1,kibana_system2,kibana_system3"),
					},
				},
			},
			want: ProvidedUser{
				name:     "ns2-kibana-sample-kibana-user",
				password: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
				roles:    []string{"kibana_system1", "kibana_system2", "kibana_system3"},
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
						PasswordHash: []byte("$2a$10$D6q/zdYfGJsJxipsZ4Jioul8tWIcL.o.Mhx/as1nlNdOX6EgqRRRS"),
						UserRoles:    []byte("kibana_system"),
					},
				},
			},
			want: ProvidedUser{},
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
						UserName:  []byte("ns2-kibana-sample-kibana-user"),
						UserRoles: []byte("kibana_system"),
					},
				},
			},
			want: ProvidedUser{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProvidedUserFromSecret(tt.args.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvidedUserFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewProvidedUserFromSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
