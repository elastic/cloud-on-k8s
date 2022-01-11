// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package manager

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func ownedSecret(namespace, name, ownerNs, ownerName, ownerKind string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{
			reconciler.SoftOwnerNameLabel:      ownerName,
			reconciler.SoftOwnerNamespaceLabel: ownerNs,
			reconciler.SoftOwnerKindLabel:      ownerKind,
		}}}
}

//nolint:thelper
func Test_garbageCollectSoftOwnedSecrets(t *testing.T) {
	log = logf.Log.WithName("test")
	tests := []struct {
		name        string
		runtimeObjs []runtime.Object
		assert      func(c k8s.Client, t *testing.T)
	}{
		{
			name: "don't gc secrets owned by a different Kind of resource",
			runtimeObjs: []runtime.Object{
				// secret referencing another resource (a Secret) that does not exist anymore
				ownedSecret("ns", "secret-1", "ns", "a-secret", "Secret"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// secret still there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{}))
			},
		},
		{
			name: "no Elasticsearch soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					TypeMeta:   metav1.TypeMeta{Kind: "Elasticsearch"},
				},
				ownedSecret("ns", "secret-1", "ns", "es", "Elasticsearch"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// es + the secret are still there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "es"}, &esv1.Elasticsearch{}))
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{}))
			},
		},
		{
			name: "some Elasticsearch soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				// secret referencing ES that does not exist anymore
				ownedSecret("ns", "secret-1", "ns", "es", "Elasticsearch"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// secret has been removed
				require.True(t, apierrors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{})))
			},
		},
		{
			name: "no Kibana soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				&kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					TypeMeta:   metav1.TypeMeta{Kind: "Kibana"},
				},
				ownedSecret("ns", "secret-1", "ns", "es", "Kibana"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// kibana + the secret are still there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "es"}, &kbv1.Kibana{}))
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{}))
			},
		},
		{
			name: "some Kibana soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				// secret referencing Kibana that does not exist anymore
				ownedSecret("ns", "secret-1", "ns", "es", "Kibana"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// secret has been removed
				require.True(t, apierrors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{})))
			},
		},
		{
			name: "no ApmServer soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				&apmv1.ApmServer{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					TypeMeta:   metav1.TypeMeta{Kind: "ApmServer"},
				},
				ownedSecret("ns", "secret-1", "ns", "es", "ApmServer"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// ApmServer + the secret are still there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "es"}, &apmv1.ApmServer{}))
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{}))
			},
		},
		{
			name: "some ApmServer soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				// secret referencing ApmServer that does not exist anymore
				ownedSecret("ns", "secret-1", "ns", "es", "ApmServer"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// secret has been removed
				require.True(t, apierrors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{})))
			},
		},
		{
			name: "no EnterpriseSearch soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				&entv1.EnterpriseSearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					TypeMeta:   metav1.TypeMeta{Kind: "EnterpriseSearch"},
				},
				ownedSecret("ns", "secret-1", "ns", "es", "EnterpriseSearch"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// EnterpriseSearch + the secret are still there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "es"}, &entv1.EnterpriseSearch{}))
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{}))
			},
		},
		{
			name: "some EnterpriseSearch soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				// secret referencing EnterpriseSearch that does not exist anymore
				ownedSecret("ns", "secret-1", "ns", "es", "EnterpriseSearch"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// secret has been removed
				require.True(t, apierrors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{})))
			},
		},
		{
			name: "no Beat soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				&beatv1beta1.Beat{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					TypeMeta:   metav1.TypeMeta{Kind: "Beat"},
				},
				ownedSecret("ns", "secret-1", "ns", "es", "Beat"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// Beat + the secret are still there
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "es"}, &beatv1beta1.Beat{}))
				require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{}))
			},
		},
		{
			name: "some Beat soft-owned secrets to gc",
			runtimeObjs: []runtime.Object{
				// secret referencing Beat that does not exist anymore
				ownedSecret("ns", "secret-1", "ns", "es", "Beat"),
			},
			assert: func(c k8s.Client, t *testing.T) {
				// secret has been removed
				require.True(t, apierrors.IsNotFound(c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret-1"}, &corev1.Secret{})))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.runtimeObjs...)
			garbageCollectSoftOwnedSecrets(c)
			tt.assert(c, t)
		})
	}
}

func Test_determineSetDefaultSecurityContext(t *testing.T) {
	type args struct {
		setDefaultSecurityContext string
		clientset                 kubernetes.Interface
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			"auto-detect on OpenShift cluster does not set security context",
			args{
				"auto-detect",
				newFakeK8sClientsetWithDiscovery([]*metav1.APIResourceList{
					{
						GroupVersion: schema.GroupVersion{Group: "security.openshift.io", Version: "v1"}.String(),
						APIResources: []metav1.APIResource{
							{
								Name: "securitycontextconstraints",
							},
						},
					},
				}, nil),
			},
			false,
			false,
		},
		{
			"auto-detect on OpenShift cluster, returning group discovery failed error for OpenShift security group+version, does not set security context",
			args{
				"auto-detect",
				newFakeK8sClientsetWithDiscovery([]*metav1.APIResourceList{}, &discovery.ErrGroupDiscoveryFailed{
					Groups: map[schema.GroupVersion]error{
						{Group: "security.openshift.io", Version: "v1"}: nil,
					},
				}),
			},
			false,
			false,
		},
		{
			"auto-detect on non-OpenShift cluster, returning not found error, sets security context",
			args{
				"auto-detect",
				newFakeK8sClientsetWithDiscovery([]*metav1.APIResourceList{}, apierrors.NewNotFound(schema.GroupResource{
					Group:    "security.openshift.io",
					Resource: "none",
				}, "fake")),
			},
			true,
			false,
		},
		{
			"auto-detect on non-OpenShift cluster, returning random error, returns error",
			args{
				"auto-detect",
				newFakeK8sClientsetWithDiscovery([]*metav1.APIResourceList{}, fmt.Errorf("random error")),
			},
			true,
			true,
		},
		{
			"true set, returning no error, will set security context",
			args{
				"true",
				newFakeK8sClientsetWithDiscovery([]*metav1.APIResourceList{}, nil),
			},
			true,
			false,
		}, {
			"false set, returning no error, will not set security context",
			args{
				"false",
				newFakeK8sClientsetWithDiscovery([]*metav1.APIResourceList{}, nil),
			},
			false,
			false,
		}, {
			"invalid bool set, returns error",
			args{
				"invalid",
				newFakeK8sClientsetWithDiscovery([]*metav1.APIResourceList{}, nil),
			},
			false,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := determineSetDefaultSecurityContext(tt.args.setDefaultSecurityContext, tt.args.clientset)
			if (err != nil) != tt.wantErr {
				t.Errorf("determineSetDefaultSecurityContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("determineSetDefaultSecurityContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeClientset struct {
	kubernetes.Interface
	discovery discovery.DiscoveryInterface
}

type fakeDiscovery struct {
	discovery.DiscoveryInterface
	resources                         []*metav1.APIResourceList
	errServerResourcesForGroupVersion error
}

func (c *fakeClientset) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

func (d *fakeDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	if d.errServerResourcesForGroupVersion != nil {
		return nil, d.errServerResourcesForGroupVersion
	}
	for _, resourceList := range d.resources {
		if resourceList.GroupVersion == groupVersion {
			return resourceList, nil
		}
	}
	return nil, fmt.Errorf("GroupVersion %q not found", groupVersion)
}

func newFakeK8sClientsetWithDiscovery(resources []*metav1.APIResourceList, discoveryError error) kubernetes.Interface {
	discoveryClient := &fakeDiscovery{
		resources:                         resources,
		errServerResourcesForGroupVersion: discoveryError,
	}
	client := &fakeClientset{
		discovery: discoveryClient,
	}
	return client
}
