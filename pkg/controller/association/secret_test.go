// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestGetUnmanagedAssociationConnexionInfoFromSecret(t *testing.T) {
	type args struct {
		c func() k8s.Client
	}
	refObjectSelector := commonv1.ObjectSelector{Namespace: "a", Name: "b"}
	unmanagedRefSecretFixture := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "b"},
		Data: map[string][]byte{
			"url":      []byte("https://es.io:9243"),
			"username": []byte("elastic"),
			"password": []byte("elastic"),
		},
	}
	refObjectFixture := UnmanagedAssociationConnectionInfo{URL: "https://es.io:9243", Username: "elastic", Password: "elastic", CaCert: ""}

	tests := []struct {
		name    string
		args    args
		want    func() UnmanagedAssociationConnectionInfo
		wantErr bool
	}{
		{
			name: "happy path",
			args: args{
				c: func() k8s.Client { return k8s.NewFakeClient(unmanagedRefSecretFixture) },
			},
			want:    func() UnmanagedAssociationConnectionInfo { return refObjectFixture },
			wantErr: false,
		},
		{
			name: "happy path with a ca",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					secretCopy.Data["ca.crt"] = []byte("XXXXXXXXXXXX")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			want: func() UnmanagedAssociationConnectionInfo {
				o := refObjectFixture
				o.CaCert = "XXXXXXXXXXXX"
				return o
			},
			wantErr: false,
		},
		{
			name: "secret does not exist",
			args: args{
				c: func() k8s.Client { return k8s.NewFakeClient() },
			},
			wantErr: true,
		},
		{
			name: "invalid secret: missing url",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					delete(secretCopy.Data, "url")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			wantErr: true,
		},
		{
			name: "invalid secret: missing username",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					delete(secretCopy.Data, "username")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			wantErr: true,
		},
		{
			name: "invalid secret: missing password",
			args: args{
				c: func() k8s.Client {
					secretCopy := unmanagedRefSecretFixture.DeepCopy()
					delete(secretCopy.Data, "password")
					return k8s.NewFakeClient(secretCopy)
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUnmanagedAssociationConnectionInfoFromSecret(tt.args.c(), refObjectSelector)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetUnmanagedAssociationConnectionInfoFromSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil && *got != tt.want() {
				t.Errorf("GetUnmanagedAssociationConnectionInfoFromSecret() got = %v, want %v", *got, tt.want())
			}
		})
	}
}
