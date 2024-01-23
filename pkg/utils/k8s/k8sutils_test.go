// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"net"
	"reflect"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	netutil "github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

func TestDeepCopyObject(t *testing.T) {
	testCases := []struct {
		name string
		obj  client.Object
		want client.Object
	}{
		{
			name: "nil input",
		},
		{
			name: "valid object",
			obj:  &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}},
			want: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := DeepCopyObject(tc.obj)
			require.Equal(t, tc.want, have)
			require.True(t, &tc.want != &have, "Copied object has the same memory location")
		})
	}
}

func TestToObjectMeta(t *testing.T) {
	assert.Equal(
		t,
		metav1.ObjectMeta{Namespace: "namespace", Name: "name"},
		ToObjectMeta(types.NamespacedName{Namespace: "namespace", Name: "name"}),
	)
}

func TestExtractNamespacedName(t *testing.T) {
	assert.Equal(
		t,
		types.NamespacedName{Namespace: "namespace", Name: "name"},
		ExtractNamespacedName(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace", Name: "name"}}),
	)
}

func TestGetServiceDNSName(t *testing.T) {
	type args struct {
		svc corev1.Service
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "sample service",
			args: args{
				svc: corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "test-name"}},
			},
			want: []string{"test-name.test-ns.svc", "test-name.test-ns"},
		},
		{
			name: "load balancer service",
			args: args{
				svc: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "test-name"},
					Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
					Status:     corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{Hostname: "mysvc.lb"}}}},
				},
			},
			want: []string{"test-name.test-ns.svc", "test-name.test-ns", "mysvc.lb"},
		},
		{
			name: "load balancer service (no status)",
			args: args{
				svc: corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "test-name"},
					Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
				},
			},
			want: []string{"test-name.test-ns.svc", "test-name.test-ns"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := deep.Equal(GetServiceDNSName(tt.args.svc), tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestGetServiceIPAddresses(t *testing.T) {
	testCases := []struct {
		name string
		svc  corev1.Service
		want []net.IP
	}{
		{
			name: "ClusterIP service",
			svc:  corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}},
			want: nil,
		},
		{
			name: "NodePort service with external IP addresses",
			svc:  corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, ExternalIPs: []string{"1.2.3.4", "2001:db8:a0b:12f0::1"}}},
			want: []net.IP{netutil.IPToRFCForm(net.ParseIP("1.2.3.4")), netutil.IPToRFCForm(net.ParseIP("2001:db8:a0b:12f0::1"))},
		},
		{
			name: "LoadBalancer service",
			svc: corev1.Service{
				Spec:   corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
				Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}}}},
			},
			want: []net.IP{netutil.IPToRFCForm(net.ParseIP("1.2.3.4"))},
		},
		{
			name: "LoadBalancer service (no status)",
			svc:  corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}},
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := GetServiceIPAddresses(tc.svc)
			require.Equal(t, tc.want, have)
		})
	}
}

func TestCompareStorageRequests(t *testing.T) {
	type args struct {
		initial corev1.VolumeResourceRequirements
		updated corev1.VolumeResourceRequirements
	}
	tests := []struct {
		name string
		args args
		want StorageComparison
	}{
		{
			name: "same size",
			args: args{
				initial: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				}},
				updated: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				}},
			},
			want: StorageComparison{},
		},
		{
			name: "storage increase",
			args: args{
				initial: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				}},
				updated: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("2Gi"),
				}},
			},
			want: StorageComparison{Increase: true},
		},
		{
			name: "storage decrease",
			args: args{
				initial: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("2Gi"),
				}},
				updated: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				}},
			},
			want: StorageComparison{Decrease: true},
		},
		{
			name: "no storage specified in both",
			args: args{
				initial: corev1.VolumeResourceRequirements{},
				updated: corev1.VolumeResourceRequirements{},
			},
			want: StorageComparison{},
		},
		{
			name: "no initial storage specified: not an increase",
			args: args{
				initial: corev1.VolumeResourceRequirements{},
				updated: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				}},
			},
			want: StorageComparison{},
		},
		{
			name: "no updated storage specified: not a decrease",
			args: args{
				initial: corev1.VolumeResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				}},
				updated: corev1.VolumeResourceRequirements{},
			},
			want: StorageComparison{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareStorageRequests(tt.args.initial, tt.args.updated); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CompareStorageRequests() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObjectExists(t *testing.T) {
	type args struct {
		c             Client
		ref           types.NamespacedName
		typedReceiver client.Object
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "existing secret",
			args: args{
				c: NewFakeClient(
					&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "secret-name"}},
				),
				ref:           types.NamespacedName{Namespace: "ns", Name: "secret-name"},
				typedReceiver: &corev1.Secret{},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "non-existing secret",
			args: args{
				c: NewFakeClient(
					&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "secret-name"}},
				),
				ref:           types.NamespacedName{Namespace: "ns", Name: "another-secret-name"},
				typedReceiver: &corev1.Secret{},
			},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ObjectExists(tt.args.c, tt.args.ref, tt.args.typedReceiver)
			if (err != nil) != tt.wantErr {
				t.Errorf("ObjectExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ObjectExists() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSecretEntriesCount(t *testing.T) {
	secretFixture := corev1.Secret{Data: map[string][]byte{
		"a": nil,
		"b": nil,
		"c": nil,
	}}
	type args struct {
		secret corev1.Secret
		keys   []string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "empty secret",
			args: args{
				secret: corev1.Secret{},
				keys:   []string{"a"},
			},
			want: 0,
		},
		{
			name: "empty keys",
			args: args{
				secret: secretFixture,
				keys:   nil,
			},
			want: 0,
		},
		{
			name: "single key",
			args: args{
				secret: secretFixture,
				keys:   []string{"a"},
			},
			want: 1,
		},
		{
			name: "multiple keys",
			args: args{
				secret: secretFixture,
				keys:   []string{"a", "c"},
			},
			want: 2,
		},
		{
			name: "no match single",
			args: args{
				secret: secretFixture,
				keys:   []string{"d"},
			},
			want: 0,
		},
		{
			name: "partial match multiple",
			args: args{
				secret: secretFixture,
				keys:   []string{"a", "f"},
			},
			want: 1,
		},
		{
			name: "match all",
			args: args{
				secret: secretFixture,
				keys:   []string{"a", "b", "c"},
			},
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, GetSecretEntriesCount(tt.args.secret, tt.args.keys...), "GetSecretEntriesCount(%v, %v)", tt.args.secret, tt.args.keys)
		})
	}
}
