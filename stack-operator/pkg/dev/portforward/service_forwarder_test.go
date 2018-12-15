package portforward

import (
	"context"
	"net"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_parseServiceAddr(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 string
	}{
		{
			name:  "service with namespace",
			args:  args{addr: "foo.bar.svc.cluster.local"},
			want:  "foo",
			want1: "bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := parseServiceAddr(tt.args.addr)
			if got != tt.want {
				t.Errorf("parseServiceAddr() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("parseServiceAddr() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

type capturingPodForwarderFactory struct {
	addrs []string
}

func (f *capturingPodForwarderFactory) NewPodForwarder(network, addr string) Forwarder {
	f.addrs = append(f.addrs, addr)
	return &stubForwarder{network: network, addr: addr}
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
		name            string
		fields          fields
		tweaks          func(f *serviceForwarder)
		args            args
		want            net.Conn
		wantErr         bool
		extraAssertions func(t *testing.T, tt test, f *serviceForwarder)
	}

	tests := []test{
		{
			name: "should forward to a ready pod",
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
							Selector: map[string]string{"foo": "bar"},
							Ports: []v1.ServicePort{
								{
									Port:       9200,
									TargetPort: intstr.FromInt(9200),
								},
							},
						},
					},
					&v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "some-pod-name",
							Namespace: "bar",
							Labels:    map[string]string{"foo": "bar"},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Ports: []v1.ContainerPort{
										{
											Name:          "http",
											ContainerPort: 9200,
										},
									},
								},
							},
						},
						Status: v1.PodStatus{
							Conditions: []v1.PodCondition{
								{Type: v1.PodReady, Status: v1.ConditionTrue},
								{Type: v1.ContainersReady, Status: v1.ConditionTrue},
							},
						},
					},
				),
			},
			tweaks: func(f *serviceForwarder) {
				f.podForwarderFactory = &capturingPodForwarderFactory{}
			},
			extraAssertions: func(t *testing.T, tt test, f *serviceForwarder) {
				ff := f.podForwarderFactory.(*capturingPodForwarderFactory)
				assert.Equal(t, []string{"some-pod-name.bar.pod.cluster.local:http"}, ff.addrs)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewServiceForwarder(tt.fields.client, tt.fields.network, tt.fields.addr)

			if tt.tweaks != nil {
				tt.tweaks(f)
			}

			got, err := f.DialContext(tt.args.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("serviceForwarder.DialContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("serviceForwarder.DialContext() = %v, want %v", got, tt.want)
			}

			if tt.extraAssertions != nil {
				tt.extraAssertions(t, tt, f)
			}
		})
	}
}
