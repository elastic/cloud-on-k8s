// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package portforward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_parseServiceAddr(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr error
	}{
		{
			name: "service with namespace",
			args: args{addr: "foo.bar.svc"},
			want: types.NamespacedName{Namespace: "bar", Name: "foo"},
		},
		{
			name:    "non-fqdn service name",
			args:    args{addr: "foo"},
			wantErr: errors.New("unsupported service address format: foo"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServiceAddr(tt.args.addr)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.want, *got)
		})
	}
}

func Test_serviceForwarder_DialContext(t *testing.T) {
	type fields struct {
		client  client.Client
		network string
		addr    string
	}
	type args struct {
		ctx context.Context
	}
	type test struct {
		name    string
		fields  fields
		tweaks  func(f *ServiceForwarder)
		args    args
		want    net.Conn
		wantErr error
	}

	tests := []test{
		{
			name: "should forward to a ready endpoint address with Kind=Pod",
			fields: fields{
				network: "tcp",
				addr:    "foo.bar.svc:9200",
				client: k8s.NewFakeClient(
					&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{
								{
									Port:       9200,
									TargetPort: intstr.FromInt(9200),
								},
							},
						},
					},
					&corev1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Subsets: []corev1.EndpointSubset{
							{
								Ports: []corev1.EndpointPort{{Port: 9200}},
								Addresses: []corev1.EndpointAddress{
									{
										TargetRef: &corev1.ObjectReference{
											Kind:      "Pod",
											Name:      "some-pod-name",
											Namespace: "bar",
										},
									},
								},
							},
						},
					},
				),
			},
			tweaks: func(f *ServiceForwarder) {
				f.podForwarderFactory = func(_ context.Context, network, addr string) (Forwarder, error) {
					return &stubForwarder{
						onDialContext: func(ctx context.Context) (net.Conn, error) {
							return nil, fmt.Errorf("would dial: %s", addr)
						},
					}, nil
				}
			},
			wantErr: errors.New("would dial: some-pod-name.bar.pod:9200"),
		},
		{
			name: "should fail if the service is not listening on the specified port",
			fields: fields{
				network: "tcp",
				addr:    "foo.bar.svc:1234",
				client: k8s.NewFakeClient(
					&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
					},
				),
			},
			wantErr: errors.New("service is not listening on port: 1234"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewServiceForwarder(tt.fields.client, tt.fields.network, tt.fields.addr)
			assert.NoError(t, err)

			if tt.tweaks != nil {
				tt.tweaks(f)
			}

			got, err := f.DialContext(tt.args.ctx)
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
