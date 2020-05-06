// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

var (
	varTrue = true

	// Throughout those tests we'll use Kibana association for testing purposes,
	// but tests are the same for any resource type.
	kbAssociationInfo = AssociationInfo{
		AssociationObjTemplate: func() commonv1.Association { return &kbv1.Kibana{} },
		AssociationName:        "kb-es",
		AssociatedShortName:    "kb",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				"kibanaassociation.k8s.elastic.co/name":      associated.Name,
				"kibanaassociation.k8s.elastic.co/namespace": associated.Namespace,
			}
		},
		UserSecretSuffix: "kibana-user",
		ESUserRole: func(associated commonv1.Associated) (string, error) {
			return "kibana_system", nil
		},
		AssociatedNamer: esv1.ESNamer,
		ElasticsearchRef: func(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
			return true, association.AssociationRef(), nil
		},
		ExternalServiceURL: func(c k8s.Client, association commonv1.Association) (string, error) {
			esRef := association.AssociationRef()
			es := esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: esRef.Namespace, Name: esRef.Name},
			}
			return services.ExternalServiceURL(es), nil
		},
	}

	kibanaNamespace     = "kbns"
	esNamespace         = "esns"
	sampleES            = esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: esNamespace, Name: "esname"}}
	sampleKibanaNoEsRef = func() kbv1.Kibana {
		return kbv1.Kibana{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: kibanaNamespace,
				Name:      "kbname",
			},
		}
	}
	sampleKibanaWithESRef = func() kbv1.Kibana {
		sample := sampleKibanaNoEsRef()
		kb := (&sample).DeepCopy()
		kb.Spec = kbv1.KibanaSpec{ElasticsearchRef: commonv1.ObjectSelector{Name: sampleES.Name, Namespace: sampleES.Namespace}}
		return *kb
	}
	sampleAssociatedKibana = func() kbv1.Kibana {
		sample := sampleKibanaWithESRef()
		kb := (&sample).DeepCopy()
		kb.Annotations = map[string]string{
			kb.AnnotationName(): "{\"authSecretName\":\"kbname-kibana-user\",\"authSecretKey\":\"kbns-kbname-kibana-user\",\"caCertProvided\":true,\"caSecretName\":\"kbname-kb-es-ca\",\"url\":\"https://esname-es-http.esns.svc:9200\"}",
		}
		return *kb
	}
	kbNamespacedName = types.NamespacedName{Namespace: kibanaNamespace, Name: sampleKibanaWithESRef().Name}

	// es public http certs containing the ca cert to be trusted
	esHTTPPublicCertsSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "esname-es-http-certs-public",
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("ca cert content"),
			"tls.crt": []byte("tls cert content"),
		},
	}
	// kibana user in the ES namespace created for the association
	kibanaUserInESNamespace = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "kbns-kbname-kibana-user",
			Labels: map[string]string{
				"common.k8s.elastic.co/type":                 "user",
				"elasticsearch.k8s.elastic.co/cluster-name":  "esname",
				"kibanaassociation.k8s.elastic.co/name":      "kbname",
				"kibanaassociation.k8s.elastic.co/namespace": "kbns",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "elasticsearch.k8s.elastic.co/v1",
					Kind:               "Elasticsearch",
					Name:               "esname",
					Controller:         &varTrue,
					BlockOwnerDeletion: &varTrue,
				},
			},
		},
		Data: map[string][]byte{
			"name":         []byte("kbns-kbname-kibana-user"),
			"passwordHash": []byte("$2a$10$7WSe8NagB3MTI/RdP4Gk5uHJJTJ4ZCrPfd0G9DmDsjGCJLRTur6Di"),
			"userRoles":    []byte("kibana_system"),
		},
	}
	// es public certs we expect to be copied over into the Kibana namespace
	esCertsInKibanaNamespace = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kibanaNamespace,
			Name:      "kbname-kb-es-ca",
			Labels: map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name":  "esname",
				"kibanaassociation.k8s.elastic.co/name":      "kbname",
				"kibanaassociation.k8s.elastic.co/namespace": "kbns",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "kibana.k8s.elastic.co/v1",
					Kind:               "Kibana",
					Name:               "kbname",
					Controller:         &varTrue,
					BlockOwnerDeletion: &varTrue,
				},
			},
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("ca cert content"),
			"tls.crt": []byte("tls cert content"),
		},
	}
	// kibana user credentials in the Kibana namespace
	kibanaUserInKibanaNamespace = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: kibanaNamespace,
			Name:      "kbname-kibana-user",
			Labels: map[string]string{
				"eck.k8s.elastic.co/credentials":             "true",
				"elasticsearch.k8s.elastic.co/cluster-name":  "esname",
				"kibanaassociation.k8s.elastic.co/name":      "kbname",
				"kibanaassociation.k8s.elastic.co/namespace": "kbns",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "kibana.k8s.elastic.co/v1",
					Kind:               "Kibana",
					Name:               "kbname",
					Controller:         &varTrue,
					BlockOwnerDeletion: &varTrue,
				},
			},
		},
		Data: map[string][]byte{
			"kbns-kbname-kibana-user": []byte("cXEyeHd4dDhmNGNqenZ0Y2RjNzhnaGpx"),
		},
	}
	setDynamicWatches = func(t *testing.T, r Reconciler, kb kbv1.Kibana) {
		err := r.setUserAndCaWatches(&kb, kb.Spec.ElasticsearchRef.NamespacedName(), kb.Spec.ElasticsearchRef.NamespacedName(), esv1.ESNamer)
		require.NoError(t, err)
	}
)

type denyAllAccessReviewer struct{}

func (a denyAllAccessReviewer) AccessAllowed(serviceAccount string, sourceNamespace string, object runtime.Object) (bool, error) {
	return false, nil
}

func testReconciler(runtimeObjs ...runtime.Object) Reconciler {
	return Reconciler{
		AssociationInfo: kbAssociationInfo,
		Client:          k8s.WrappedFakeClient(runtimeObjs...),
		accessReviewer:  rbac.NewPermissiveAccessReviewer(),
		watches:         watches.NewDynamicWatches(),
		recorder:        record.NewFakeRecorder(10),
		Parameters: operator.Parameters{
			OperatorInfo: about.OperatorInfo{
				BuildInfo: about.BuildInfo{
					Version: "unit-tests",
				},
			},
		},
		logger: log.WithName("test"),
	}
}

func TestReconciler_Reconcile_resourceNotFound(t *testing.T) {
	// no resource in the apiserver
	r := testReconciler()
	// should do nothing
	res, err := r.Reconcile(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "resource"}})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestReconciler_Reconcile_resourceNotFound_OnDeletion(t *testing.T) {
	// Kibana does not exist in the apiserver, but there is a leftover es user in es namespace
	r := testReconciler(&kibanaUserInESNamespace)
	res, err := r.Reconcile(reconcile.Request{NamespacedName: kbNamespacedName})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)
	// es user secret should have been removed
	var secret corev1.Secret
	err = r.Client.Get(k8s.ExtractNamespacedName(&kibanaUserInESNamespace), &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
}

func TestReconciler_Reconcile_Unmanaged(t *testing.T) {
	kb := sampleKibanaWithESRef()
	kb.Annotations = map[string]string{common.ManagedAnnotation: "false"}
	r := testReconciler(&kb)
	res, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	// should do nothing
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestReconciler_Reconcile_DeletionTimestamp(t *testing.T) {
	kb := sampleKibanaWithESRef()
	now := metav1.NewTime(time.Now())
	kb.DeletionTimestamp = &now
	r := testReconciler(&kb)
	res, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	// should do nothing
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, res)
}

func TestReconciler_Reconcile_NotCompatible(t *testing.T) {
	kb := sampleKibanaWithESRef()
	// set an incompatible controller annotation
	kb.Annotations = map[string]string{
		annotation.ControllerVersionAnnotation: "0.9.0",
	}
	r := testReconciler(&kb)
	_, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	// should error out
	require.Error(t, err)
}

func TestReconciler_Reconcile_SetsControllerVersion(t *testing.T) {
	kb := sampleKibanaWithESRef()
	r := testReconciler(&kb)
	_, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// should update the controller version annotation on Kibana
	var updatedKibana kbv1.Kibana
	err = r.Get(k8s.ExtractNamespacedName(&kb), &updatedKibana)
	require.NoError(t, err)
	require.Equal(t, "unit-tests", updatedKibana.Annotations[annotation.ControllerVersionAnnotation])
}

func TestReconciler_Reconcile_DeletesOrphanedResource(t *testing.T) {
	// setup Kibana with no ES ref,
	// and the user in es namespace that should be garbage collected
	kb := sampleKibanaNoEsRef()
	r := testReconciler(&kb, &kibanaUserInESNamespace)
	_, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// should delete the kibana user in es namespace
	var secret corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInESNamespace), &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
}

func TestReconciler_Reconcile_NoESRef_Cleanup(t *testing.T) {
	// setup Kibana with no ES ref,
	// but with a config annotation and secrets resources to clean
	kb := sampleKibanaNoEsRef()
	kb.Annotations = sampleAssociatedKibana().Annotations
	require.NotEmpty(t, kb.Annotations[kb.AnnotationName()])
	r := testReconciler(&kb, &kibanaUserInESNamespace, &kibanaUserInKibanaNamespace, &esCertsInKibanaNamespace)
	// simulate watches being set
	setDynamicWatches(t, r, sampleAssociatedKibana())
	require.NotEmpty(t, r.watches.Secrets.Registrations())
	require.NotEmpty(t, r.watches.ElasticsearchClusters.Registrations())

	_, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// should delete the kibana user in es namespace
	var secret corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInESNamespace), &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
	// should delete the kibana user in kibana namespace
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInKibanaNamespace), &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
	// should delete the es certs in kibana namespace
	err = r.Get(k8s.ExtractNamespacedName(&esCertsInKibanaNamespace), &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
	// should remove the association conf in annotations
	var updatedKibana kbv1.Kibana
	err = r.Get(k8s.ExtractNamespacedName(&kb), &updatedKibana)
	require.NoError(t, err)
	require.Empty(t, updatedKibana.Annotations[kb.AnnotationName()])
	// should remove dynamic watches
	require.Empty(t, r.watches.Secrets.Registrations())
	require.Empty(t, r.watches.ElasticsearchClusters.Registrations())
}

func TestReconciler_Reconcile_NoES(t *testing.T) {
	kb := sampleAssociatedKibana()
	require.NotEmpty(t, kb.Annotations[kb.AnnotationName()])
	// es resource does not exist
	r := testReconciler(&kb)
	_, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// association status should become pending
	var updatedKibana kbv1.Kibana
	err = r.Get(k8s.ExtractNamespacedName(&kb), &updatedKibana)
	require.NoError(t, err)
	require.Equal(t, commonv1.AssociationPending, updatedKibana.Status.AssociationStatus)
	// association conf should have been removed
	require.Empty(t, updatedKibana.Annotations[kb.AnnotationName()])
}

func TestReconciler_Reconcile_RBACNotAllowed(t *testing.T) {
	kb := sampleAssociatedKibana()
	require.NotEmpty(t, kb.Annotations[kb.AnnotationName()])
	r := testReconciler(&kb, &sampleES, &kibanaUserInESNamespace)
	// simulate rbac association disallowed
	r.accessReviewer = denyAllAccessReviewer{}
	_, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// association should be pending
	var updatedKibana kbv1.Kibana
	err = r.Get(k8s.ExtractNamespacedName(&kb), &updatedKibana)
	require.NoError(t, err)
	require.Equal(t, commonv1.AssociationPending, updatedKibana.Status.AssociationStatus)
	// association conf should be removed
	require.Empty(t, updatedKibana.Annotations[kb.AnnotationName()])
	// user in es namespace should be deleted
	var secret corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInESNamespace), &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
}

func TestReconciler_Reconcile_NewAssociation(t *testing.T) {
	// Kibana references ES, but no secret nor association conf exist yet
	kb := sampleKibanaWithESRef()
	require.Empty(t, kb.Annotations[kb.AnnotationName()])
	r := testReconciler(&kb, &sampleES, &esHTTPPublicCertsSecret)
	// no resources are watched yet
	require.Empty(t, r.watches.Secrets.Registrations())
	require.Empty(t, r.watches.ElasticsearchClusters.Registrations())
	// run the reconciliation
	results, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// no requeue to trigger
	require.Equal(t, reconcile.Result{}, results)

	// should create the kibana user in es namespace
	var actualKbUserInESNamespace corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInESNamespace), &actualKbUserInESNamespace)
	require.NoError(t, err)
	// password hash should be generated so let's ignore its exact content in the comparison
	require.NotEmpty(t, actualKbUserInESNamespace.Data[user.PasswordHashField])
	expected := kibanaUserInESNamespace.DeepCopy()
	expected.Data[user.PasswordHashField] = actualKbUserInESNamespace.Data[user.PasswordHashField]
	comparison.RequireEqual(t, expected, &actualKbUserInESNamespace)

	// should create the kibana user in kibana namespace
	var actualKbUserInKbNamespace corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInKibanaNamespace), &actualKbUserInKbNamespace)
	require.NoError(t, err)
	// password should be generated so let's ignore its exact content in the comparison
	require.NotEmpty(t, actualKbUserInKbNamespace.Data)
	expected = kibanaUserInKibanaNamespace.DeepCopy()
	expected.Data = actualKbUserInKbNamespace.Data
	comparison.RequireEqual(t, expected, &actualKbUserInKbNamespace)

	// should create the es certs in kibana namespace
	var actualEsCertsInKibanaNamespace corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&esCertsInKibanaNamespace), &actualEsCertsInKibanaNamespace)
	require.NoError(t, err)
	comparison.RequireEqual(t, &esCertsInKibanaNamespace, &actualEsCertsInKibanaNamespace)

	// should have dynamic watches set
	require.NotEmpty(t, r.watches.Secrets.Registrations())
	require.NotEmpty(t, r.watches.ElasticsearchClusters.Registrations())

	var updatedKibana kbv1.Kibana
	err = r.Get(k8s.ExtractNamespacedName(&kb), &updatedKibana)
	// association conf should be set
	require.Equal(t, sampleAssociatedKibana().Annotations[kb.AnnotationName()], updatedKibana.Annotations[kb.AnnotationName()])
	// association status should be established
	require.NoError(t, err)
	require.Equal(t, commonv1.AssociationEstablished, updatedKibana.Status.AssociationStatus)
}

func TestReconciler_Reconcile_ExistingAssociation_NoOp(t *testing.T) {
	// association already established, reconciliation should be a no-op
	kb := sampleAssociatedKibana()
	r := testReconciler(&kb, &sampleES, &kibanaUserInESNamespace, &kibanaUserInKibanaNamespace, &esHTTPPublicCertsSecret, &esCertsInKibanaNamespace)
	// run the reconciliation
	results, err := r.Reconcile(reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// no requeue to trigger
	require.Equal(t, reconcile.Result{}, results)

	// same kibana user in es namespace
	var actualKbUserInESNamespace corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInESNamespace), &actualKbUserInESNamespace)
	require.NoError(t, err)
	comparison.RequireEqual(t, &kibanaUserInESNamespace, &actualKbUserInESNamespace)

	// same kibana user in kibana namespace
	var actualKbUserInKbNamespace corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&kibanaUserInKibanaNamespace), &actualKbUserInKbNamespace)
	require.NoError(t, err)
	comparison.RequireEqual(t, &kibanaUserInKibanaNamespace, &actualKbUserInKbNamespace)

	// same the es certs in kibana namespace
	var actualEsCertsInKibanaNamespace corev1.Secret
	err = r.Get(k8s.ExtractNamespacedName(&esCertsInKibanaNamespace), &actualEsCertsInKibanaNamespace)
	require.NoError(t, err)
	comparison.RequireEqual(t, &esCertsInKibanaNamespace, &actualEsCertsInKibanaNamespace)

	var updatedKibana kbv1.Kibana
	err = r.Get(k8s.ExtractNamespacedName(&kb), &updatedKibana)
	// association conf should be set
	require.Equal(t, sampleAssociatedKibana().Annotations[kb.AnnotationName()], updatedKibana.Annotations[kb.AnnotationName()])
	// association status should be established
	require.NoError(t, err)
	require.Equal(t, commonv1.AssociationEstablished, updatedKibana.Status.AssociationStatus)
}

func TestReconciler_getElasticsearch(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	associatedKibana := kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "kb",
			Annotations: map[string]string{
				"association.k8s.elastic.co/es-conf": "association-conf-data", // we don't care about the data here
			},
		},
		Spec: kbv1.KibanaSpec{ElasticsearchRef: commonv1.ObjectSelector{Name: "es", Namespace: "ns"}},
	}
	tests := []struct {
		name              string
		runtimeObjects    []runtime.Object
		associated        commonv1.Association
		esRef             commonv1.ObjectSelector
		wantES            esv1.Elasticsearch
		wantStatus        commonv1.AssociationStatus
		wantUpdatedKibana kbv1.Kibana
	}{
		{
			name:              "retrieve existing Elasticsearch",
			runtimeObjects:    []runtime.Object{&es, &associatedKibana},
			associated:        &associatedKibana,
			esRef:             commonv1.ObjectSelector{Namespace: "ns", Name: "es"},
			wantES:            es,
			wantStatus:        "",
			wantUpdatedKibana: associatedKibana,
		},
		{
			name:           "Elasticsearch not found: remove association conf in Kibana",
			runtimeObjects: []runtime.Object{&associatedKibana}, // no ES
			associated:     &associatedKibana,
			esRef:          commonv1.ObjectSelector{Namespace: "ns", Name: "es"},
			wantES:         esv1.Elasticsearch{},
			wantStatus:     commonv1.AssociationPending,
			wantUpdatedKibana: kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "kb",
				},
				Spec: kbv1.KibanaSpec{ElasticsearchRef: commonv1.ObjectSelector{Name: "es", Namespace: "ns"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.runtimeObjects...)
			r := &Reconciler{Client: c, recorder: record.NewFakeRecorder(10)}
			es, status, err := r.getElasticsearch(context.Background(), tt.associated, tt.esRef)
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantES.ObjectMeta, es.ObjectMeta)

			var updatedKibana kbv1.Kibana
			err = c.Get(k8s.ExtractNamespacedName(&tt.wantUpdatedKibana), &updatedKibana)
			require.NoError(t, err)
			require.Equal(t, tt.wantUpdatedKibana.ObjectMeta.Annotations, updatedKibana.ObjectMeta.Annotations)
		})
	}
}
