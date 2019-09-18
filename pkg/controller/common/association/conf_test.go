// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"testing"

	apmv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	kbv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestFetchWithAssociation(t *testing.T) {
	t.Run("apmServer", testFetchAPMServer)
	t.Run("kibana", testFetchKibana)
}

func testFetchAPMServer(t *testing.T) {
	require.NoError(t, apmv1alpha1.AddToScheme(scheme.Scheme))

	testCases := []struct {
		name          string
		apmServer     *apmv1alpha1.ApmServer
		request       reconcile.Request
		wantOK        bool
		wantErr       bool
		wantAssocConf *commonv1alpha1.AssociationConf
	}{
		{
			name:      "with association annotation",
			apmServer: mkAPMServer(true),
			request:   reconcile.Request{NamespacedName: types.NamespacedName{Name: "apm-server-test", Namespace: "apm-ns"}},
			wantOK:    true,
			wantAssocConf: &commonv1alpha1.AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "apm-user",
				CASecretName:   "ca-secret",
				URL:            "https://es.svc:9300",
			},
		},
		{
			name:      "without association annotation",
			apmServer: mkAPMServer(false),
			request:   reconcile.Request{NamespacedName: types.NamespacedName{Name: "apm-server-test", Namespace: "apm-ns"}},
			wantOK:    true,
		},
		{
			name:      "non existent",
			apmServer: mkAPMServer(true),
			request:   reconcile.Request{NamespacedName: types.NamespacedName{Name: "some-other-apm", Namespace: "apm-ns"}},
			wantOK:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(tc.apmServer))

			var got apmv1alpha1.ApmServer
			ok, err := FetchWithAssociation(client, tc.request, &got)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, "apm-server-test", got.Name)
				require.Equal(t, "apm-ns", got.Namespace)
				require.Equal(t, "test-image", got.Spec.Image)
				require.EqualValues(t, 1, got.Spec.NodeCount)
				require.Equal(t, tc.wantAssocConf, got.AssociationConf())
			}
		})
	}
}

func mkAPMServer(withAnnotations bool) *apmv1alpha1.ApmServer {
	apmServer := &apmv1alpha1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-server-test",
			Namespace: "apm-ns",
		},
		Spec: apmv1alpha1.ApmServerSpec{
			Image:     "test-image",
			NodeCount: 1,
		},
	}

	if withAnnotations {
		apmServer.ObjectMeta.Annotations = map[string]string{
			annotation.AssociationConfAnnotation: `{"authSecretName":"auth-secret", "authSecretKey":"apm-user", "caSecretName": "ca-secret", "url":"https://es.svc:9300"}`,
		}
	}

	return apmServer
}

func testFetchKibana(t *testing.T) {
	require.NoError(t, kbv1alpha1.AddToScheme(scheme.Scheme))

	testCases := []struct {
		name          string
		kibana        *kbv1alpha1.Kibana
		request       reconcile.Request
		wantOK        bool
		wantErr       bool
		wantAssocConf *commonv1alpha1.AssociationConf
	}{
		{
			name:    "with association annotation",
			kibana:  mkKibana(true),
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}},
			wantOK:  true,
			wantAssocConf: &commonv1alpha1.AssociationConf{
				AuthSecretName: "auth-secret",
				AuthSecretKey:  "kb-user",
				CASecretName:   "ca-secret",
				URL:            "https://es.svc:9300",
			},
		},
		{
			name:    "without association annotation",
			kibana:  mkKibana(false),
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}},
			wantOK:  true,
		},
		{
			name:    "non existent",
			kibana:  mkKibana(true),
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "some-other-kb", Namespace: "kb-ns"}},
			wantOK:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := k8s.WrapClient(fake.NewFakeClient(tc.kibana))

			var got kbv1alpha1.Kibana
			ok, err := FetchWithAssociation(client, tc.request, &got)

			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, "kb-test", got.Name)
				require.Equal(t, "kb-ns", got.Namespace)
				require.Equal(t, "test-image", got.Spec.Image)
				require.EqualValues(t, 1, got.Spec.NodeCount)
				require.Equal(t, tc.wantAssocConf, got.AssociationConf())
			}
		})
	}
}

func mkKibana(withAnnotations bool) *kbv1alpha1.Kibana {
	kb := &kbv1alpha1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kb-test",
			Namespace: "kb-ns",
		},
		Spec: kbv1alpha1.KibanaSpec{
			Image:     "test-image",
			NodeCount: 1,
		},
	}

	if withAnnotations {
		kb.ObjectMeta.Annotations = map[string]string{
			annotation.AssociationConfAnnotation: `{"authSecretName":"auth-secret", "authSecretKey":"kb-user", "caSecretName": "ca-secret", "url":"https://es.svc:9300"}`,
		}
	}

	return kb
}

func TestUpdateAssociationConf(t *testing.T) {
	require.NoError(t, kbv1alpha1.AddToScheme(scheme.Scheme))
	kb := mkKibana(true)
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}}
	client := k8s.WrapClient(fake.NewFakeClient(kb))

	assocConf := &commonv1alpha1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "kb-user",
		CASecretName:   "ca-secret",
		URL:            "https://es.svc:9300",
	}

	// check the existing values
	var got kbv1alpha1.Kibana
	ok, _ := FetchWithAssociation(client, request, &got)
	require.True(t, ok)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.NodeCount)
	require.Equal(t, assocConf, got.AssociationConf())

	// update and check the new values
	newAssocConf := &commonv1alpha1.AssociationConf{
		AuthSecretName: "new-auth-secret",
		AuthSecretKey:  "new-kb-user",
		CASecretName:   "new-ca-secret",
		URL:            "https://new-es.svc:9300",
	}

	err := UpdateAssociationConf(client, &got, newAssocConf)
	require.NoError(t, err)

	ok, _ = FetchWithAssociation(client, request, &got)
	require.True(t, ok)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.NodeCount)
	require.Equal(t, newAssocConf, got.AssociationConf())
}

func TestRemoveAssociationConf(t *testing.T) {
	require.NoError(t, kbv1alpha1.AddToScheme(scheme.Scheme))
	kb := mkKibana(true)
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "kb-test", Namespace: "kb-ns"}}
	client := k8s.WrapClient(fake.NewFakeClient(kb))

	assocConf := &commonv1alpha1.AssociationConf{
		AuthSecretName: "auth-secret",
		AuthSecretKey:  "kb-user",
		CASecretName:   "ca-secret",
		URL:            "https://es.svc:9300",
	}

	// check the existing values
	var got kbv1alpha1.Kibana
	ok, _ := FetchWithAssociation(client, request, &got)
	require.True(t, ok)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.NodeCount)
	require.Equal(t, assocConf, got.AssociationConf())

	// remove and check the new values
	err := RemoveAssociationConf(client, &got)
	require.NoError(t, err)

	ok, _ = FetchWithAssociation(client, request, &got)
	require.True(t, ok)
	require.Equal(t, "kb-test", got.Name)
	require.Equal(t, "kb-ns", got.Namespace)
	require.Equal(t, "test-image", got.Spec.Image)
	require.EqualValues(t, 1, got.Spec.NodeCount)
	require.Nil(t, got.AssociationConf())
}
