package portforward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			args: args{addr: "foo.bar.svc.cluster.local"},
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
		tweaks  func(f *serviceForwarder)
		args    args
		want    net.Conn
		wantErr error
	}

	tests := []test{
		{
			name: "should forward to a ready endpoint address with Kind=Pod",
			fields: fields{
				network: "tcp",
				addr:    "foo.bar.svc.cluster.local:9200",
				client: fake.NewFakeClient(
					&v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Spec: v1.ServiceSpec{
							Ports: []v1.ServicePort{
								{
									Port:       9200,
									TargetPort: intstr.FromInt(9200),
								},
							},
						},
					},
					&v1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Subsets: []v1.EndpointSubset{
							{
								Ports: []v1.EndpointPort{{Port: 9200}},
								Addresses: []v1.EndpointAddress{
									{
										TargetRef: &v1.ObjectReference{
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
			tweaks: func(f *serviceForwarder) {
				f.podForwarderFactory = func(network, addr string) (Forwarder, error) {
					return &stubForwarder{
						onDialContext: func(ctx context.Context) (net.Conn, error) {
							return nil, fmt.Errorf("would dial: %s", addr)
						},
					}, nil
				}
			},
			wantErr: errors.New("would dial: some-pod-name.bar.pod.cluster.local:9200"),
		},
		{
			name: "should fail if the service is not listening on the specified port",
			fields: fields{
				network: "tcp",
				addr:    "foo.bar.svc.cluster.local:1234",
				client: fake.NewFakeClient(
					&v1.Service{
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
