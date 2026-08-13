// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	netutil "github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

// patchRecordingClient wraps a Client and records the raw bytes of the most recent Patch call.
type patchRecordingClient struct {
	Client
	lastPatchBody []byte
}

func (c *patchRecordingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	data, err := patch.Data(obj)
	if err != nil {
		return err
	}
	c.lastPatchBody = data
	return c.Client.Patch(ctx, obj, patch, opts...)
}

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

func TestIsLabelSelectorEmpty(t *testing.T) {
	tests := []struct {
		name     string
		selector *metav1.LabelSelector
		want     bool
	}{
		{
			name:     "nil selector",
			selector: nil,
			want:     true,
		},
		{
			name:     "empty selector",
			selector: &metav1.LabelSelector{},
			want:     true,
		},
		{
			name: "selector with match labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "es"},
			},
			want: false,
		},
		{
			name: "selector with match expressions",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "team", Operator: metav1.LabelSelectorOpExists},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLabelSelectorEmpty(tt.selector))
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

func TestNamespaceFilterFunc(t *testing.T) {
	type args struct {
		c        Client
		selector metav1.LabelSelector
	}
	tests := []struct {
		name           string
		args           args
		wantErr        bool
		testNamespaces map[string]bool // namespace -> expected filter result
	}{
		{
			name: "empty selector accepts all namespaces",
			args: args{
				c: NewFakeClient(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Labels: map[string]string{"env": "prod"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2", Labels: map[string]string{"env": "dev"}}},
				),
				selector: metav1.LabelSelector{},
			},
			wantErr: false,
			testNamespaces: map[string]bool{
				"ns1":          true,
				"ns2":          true,
				"any-ns":       true,
				"non-existent": true,
			},
		},
		{
			name: "selector with MatchLabels filters namespaces",
			args: args{
				c: NewFakeClient(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Labels: map[string]string{"env": "prod"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2", Labels: map[string]string{"env": "dev"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns3", Labels: map[string]string{"env": "prod"}}},
				),
				selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
			},
			wantErr: false,
			testNamespaces: map[string]bool{
				"ns1": true,
				"ns2": false,
				"ns3": true,
			},
		},
		{
			name: "selector with MatchExpressions filters namespaces",
			args: args{
				c: NewFakeClient(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Labels: map[string]string{"env": "prod"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2", Labels: map[string]string{"env": "dev"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns3", Labels: map[string]string{"env": "staging"}}},
				),
				selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "env",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"prod", "staging"},
						},
					},
				},
			},
			wantErr: false,
			testNamespaces: map[string]bool{
				"ns1": true,
				"ns2": false,
				"ns3": true,
			},
		},
		{
			name: "selector matches no namespaces",
			args: args{
				c: NewFakeClient(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Labels: map[string]string{"env": "prod"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2", Labels: map[string]string{"env": "dev"}}},
				),
				selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "staging"},
				},
			},
			wantErr: false,
			testNamespaces: map[string]bool{
				"ns1": false,
				"ns2": false,
			},
		},
		{
			name: "selector with both MatchLabels and MatchExpressions",
			args: args{
				c: NewFakeClient(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Labels: map[string]string{"env": "prod", "team": "platform"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2", Labels: map[string]string{"env": "prod", "team": "search"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns3", Labels: map[string]string{"env": "dev", "team": "platform"}}},
				),
				selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "team",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"platform"},
						},
					},
				},
			},
			wantErr: false,
			testNamespaces: map[string]bool{
				"ns1": true,
				"ns2": false,
				"ns3": false,
			},
		},
		{
			name: "invalid selector returns error",
			args: args{
				c: NewFakeClient(),
				selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "env",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{}, // In operator requires at least one value
						},
					},
				},
			},
			wantErr:        true,
			testNamespaces: nil,
		},
		{
			name: "no namespaces in cluster with non-empty selector",
			args: args{
				c: NewFakeClient(),
				selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
			},
			wantErr: false,
			testNamespaces: map[string]bool{
				"ns1":     false,
				"any-ns":  false,
				"default": false,
			},
		},
		{
			name: "selector with Exists operator",
			args: args{
				c: NewFakeClient(
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1", Labels: map[string]string{"env": "prod"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2", Labels: map[string]string{"team": "platform"}}},
					&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns3", Labels: map[string]string{"env": "dev"}}},
				),
				selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "env",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			wantErr: false,
			testNamespaces: map[string]bool{
				"ns1": true,
				"ns2": false,
				"ns3": true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filterFunc, err := NamespaceFilterFunc(t.Context(), tt.args.c, tt.args.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("NamespaceFilterFunc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				assert.Nil(t, filterFunc)
				return
			}
			require.NotNil(t, filterFunc, "filter function should not be nil when no error")
			for ns, expectedResult := range tt.testNamespaces {
				got := filterFunc(ns)
				assert.Equalf(t, expectedResult, got, "filterFunc(%q) = %v, want %v", ns, got, expectedResult)
			}
		})
	}
}

func TestPatchAnnotations(t *testing.T) {
	tests := []struct {
		name                 string
		initialAnnotations   map[string]string
		upsert               map[string]string
		remove               []string
		failingClientErr     error
		wantErr              bool
		wantAnnotations      map[string]string
		wantNoPatchCall      bool
		wantPatchAnnotations map[string]any
	}{
		{
			name:                 "add annotation to object with no annotations",
			initialAnnotations:   nil,
			upsert:               map[string]string{"new": "value"},
			wantAnnotations:      map[string]string{"new": "value"},
			wantPatchAnnotations: map[string]any{"new": "value"},
		},
		{
			name:                 "add annotation to object with existing annotations",
			initialAnnotations:   map[string]string{"keep": "same"},
			upsert:               map[string]string{"new": "value"},
			wantAnnotations:      map[string]string{"keep": "same", "new": "value"},
			wantPatchAnnotations: map[string]any{"new": "value"}, // "keep" must not appear in the patch
		},
		{
			name:                 "change value of an existing annotation",
			initialAnnotations:   map[string]string{"key": "old"},
			upsert:               map[string]string{"key": "new"},
			wantAnnotations:      map[string]string{"key": "new"},
			wantPatchAnnotations: map[string]any{"key": "new"},
		},
		{
			name:                 "remove one annotation, keep the others",
			initialAnnotations:   map[string]string{"keep": "same", "remove": "gone"},
			remove:               []string{"remove"},
			wantAnnotations:      map[string]string{"keep": "same"},
			wantPatchAnnotations: map[string]any{"remove": nil}, // "keep" must not appear in the patch
		},
		{
			name:                 "remove all annotations",
			initialAnnotations:   map[string]string{"a": "1", "b": "2"},
			remove:               []string{"a", "b"},
			wantAnnotations:      nil, // an emptied annotations map round-trips as nil due to the omitempty json tag
			wantPatchAnnotations: map[string]any{"a": nil, "b": nil},
		},
		{
			name:                 "add, change and remove combined in a single call",
			initialAnnotations:   map[string]string{"unchanged": "1", "changed": "old", "removed": "x"},
			upsert:               map[string]string{"changed": "new", "added": "y"},
			remove:               []string{"removed"},
			wantAnnotations:      map[string]string{"unchanged": "1", "changed": "new", "added": "y"},
			wantPatchAnnotations: map[string]any{"changed": "new", "removed": nil, "added": "y"}, // "unchanged" must not appear
		},
		{
			name:                 "adding a key with an empty string value is treated as a change",
			initialAnnotations:   nil,
			upsert:               map[string]string{"blank": ""},
			wantAnnotations:      map[string]string{"blank": ""},
			wantPatchAnnotations: map[string]any{"blank": ""},
		},
		{
			name:               "upserting the same value does not issue a patch",
			initialAnnotations: map[string]string{"key": "value"},
			upsert:             map[string]string{"key": "value"},
			wantAnnotations:    map[string]string{"key": "value"},
			wantNoPatchCall:    true,
		},
		{
			name:               "removing a key that does not exist does not issue a patch",
			initialAnnotations: map[string]string{"key": "value"},
			remove:             []string{"nonexistent"},
			wantAnnotations:    map[string]string{"key": "value"},
			wantNoPatchCall:    true,
		},
		{
			name:               "nil upsert and no removes does not issue a patch",
			initialAnnotations: map[string]string{"key": "value"},
			wantAnnotations:    map[string]string{"key": "value"},
			wantNoPatchCall:    true,
		},
		{
			name:            "remove non-existent key from nil annotations does not issue a patch",
			remove:          []string{"nonexistent"},
			wantNoPatchCall: true,
		},
		{
			name:             "client error is propagated",
			upsert:           map[string]string{"new": "value"},
			failingClientErr: errors.New("boom"),
			wantErr:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", Annotations: tc.initialAnnotations},
			}

			var inner Client
			if tc.failingClientErr != nil {
				inner = NewFailingClient(tc.failingClientErr)
			} else {
				inner = NewFakeClient(obj)
			}
			recorder := &patchRecordingClient{Client: inner}

			// Fetch obj so its ResourceVersion is populated; PatchAnnotations embeds it in the patch body.
			if tc.failingClientErr == nil {
				require.NoError(t, recorder.Get(t.Context(), types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, obj))
			}
			prePatchResourceVersion := obj.GetResourceVersion()

			err := PatchAnnotations(t.Context(), recorder, obj, tc.upsert, tc.remove...)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			fetched := &corev1.ConfigMap{}
			require.NoError(t, recorder.Get(t.Context(), types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, fetched))
			assert.Equal(t, tc.wantAnnotations, fetched.Annotations)

			if tc.wantNoPatchCall {
				assert.Equal(t, prePatchResourceVersion, fetched.ResourceVersion, "expected no patch request to be issued")
				assert.Nil(t, recorder.lastPatchBody, "expected no patch request to be issued")
				return
			}

			wantBody := map[string]any{
				"metadata": map[string]any{
					"resourceVersion": prePatchResourceVersion,
					"annotations":     tc.wantPatchAnnotations,
				},
			}
			var gotBody map[string]any
			require.NoError(t, json.Unmarshal(recorder.lastPatchBody, &gotBody))
			assert.Equal(t, wantBody, gotBody)
		})
	}
}

func TestPatchAnnotationsOptimisticLock(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"}}
	c := NewFakeClient(cm)

	stale := &corev1.ConfigMap{}
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: "test", Namespace: "ns"}, stale))

	// Someone else changes the object first, bumping its resourceVersion.
	live := &corev1.ConfigMap{}
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: "test", Namespace: "ns"}, live))
	live.Labels = map[string]string{"someone-else": "true"}
	require.NoError(t, c.Update(t.Context(), live))

	// Patching using the object fetched before that change must be rejected, since the
	// resourceVersion PatchAnnotations embeds no longer matches what's stored.
	err := PatchAnnotations(t.Context(), c, stale, map[string]string{"foo": "bar"})
	require.True(t, apierrors.IsConflict(err))
}

func TestPatchObjectFinalizers(t *testing.T) {
	tests := []struct {
		name                string
		initialFinalizers   []string
		finalizers          []string
		failingClientErr    error
		wantErr             bool
		wantFinalizers      []string
		wantNoPatchCall     bool
		wantPatchFinalizers []any
	}{
		{
			name:                "remove one finalizer, keep the others",
			initialFinalizers:   []string{"keep", "remove"},
			finalizers:          []string{"keep"},
			wantFinalizers:      []string{"keep"},
			wantPatchFinalizers: []any{"keep"},
		},
		{
			name:                "remove all finalizers",
			initialFinalizers:   []string{"a", "b"},
			finalizers:          nil,
			wantFinalizers:      nil,
			wantPatchFinalizers: nil, // patch sends "finalizers":null which removes all
		},
		{
			name:                "add a finalizer to object with existing finalizers",
			initialFinalizers:   []string{"existing"},
			finalizers:          []string{"existing", "new"},
			wantFinalizers:      []string{"existing", "new"},
			wantPatchFinalizers: []any{"existing", "new"},
		},
		{
			name:                "add a finalizer to object with no finalizers",
			initialFinalizers:   nil,
			finalizers:          []string{"new"},
			wantFinalizers:      []string{"new"},
			wantPatchFinalizers: []any{"new"},
		},
		{
			name:              "same finalizers list does not issue a patch",
			initialFinalizers: []string{"keep"},
			finalizers:        []string{"keep"},
			wantFinalizers:    []string{"keep"},
			wantNoPatchCall:   true,
		},
		{
			name:            "nil finalizers on empty object does not issue a patch",
			wantNoPatchCall: true,
		},
		{
			name:              "client error is propagated",
			initialFinalizers: []string{"finalizer"},
			finalizers:        nil,
			failingClientErr:  errors.New("boom"),
			wantErr:           true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obj := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", Finalizers: tc.initialFinalizers},
			}

			var inner Client
			if tc.failingClientErr != nil {
				inner = NewFailingClient(tc.failingClientErr)
			} else {
				inner = NewFakeClient(obj)
			}
			recorder := &patchRecordingClient{Client: inner}

			// Fetch obj so its ResourceVersion is populated; PatchObjectFinalizers embeds it in the patch body.
			if tc.failingClientErr == nil {
				require.NoError(t, recorder.Get(t.Context(), types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, obj))
			}
			preCallResourceVersion := obj.GetResourceVersion()

			err := PatchObjectFinalizers(t.Context(), recorder, obj, tc.finalizers)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			fetched := &corev1.ConfigMap{}
			require.NoError(t, recorder.Get(t.Context(), types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, fetched))
			assert.Equal(t, tc.wantFinalizers, fetched.Finalizers)

			if tc.wantNoPatchCall {
				assert.Equal(t, preCallResourceVersion, fetched.ResourceVersion, "expected no patch request to be issued")
				assert.Nil(t, recorder.lastPatchBody, "expected no patch request to be issued")
				return
			}

			// JSON unmarshal produces interface-nil for "finalizers":null, not []any(nil),
			// so coerce the typed nil to bare nil for a consistent comparison.
			var finalizersVal any
			if tc.wantPatchFinalizers != nil {
				finalizersVal = tc.wantPatchFinalizers
			}
			wantBody := map[string]any{
				"metadata": map[string]any{
					"resourceVersion": preCallResourceVersion,
					"finalizers":      finalizersVal,
				},
			}
			var gotBody map[string]any
			require.NoError(t, json.Unmarshal(recorder.lastPatchBody, &gotBody))
			assert.Equal(t, wantBody, gotBody)
		})
	}
}

func TestPatchObjectFinalizersOptimisticLock(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns", Finalizers: []string{"existing"}},
	}
	c := NewFakeClient(cm)

	stale := &corev1.ConfigMap{}
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: "test", Namespace: "ns"}, stale))

	// Someone else changes the object first, bumping its resourceVersion.
	live := &corev1.ConfigMap{}
	require.NoError(t, c.Get(t.Context(), types.NamespacedName{Name: "test", Namespace: "ns"}, live))
	live.Labels = map[string]string{"someone-else": "true"}
	require.NoError(t, c.Update(t.Context(), live))

	// Patching using the object fetched before that change must be rejected, since the
	// resourceVersion PatchObjectFinalizers embeds no longer matches what's stored.
	err := PatchObjectFinalizers(t.Context(), c, stale, nil)
	require.True(t, apierrors.IsConflict(err))
}
