// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestFilterWithUserProvidedClientCert(t *testing.T) {
	kbWithUserCert := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "kb-with-cert"},
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ElasticsearchSelector{
				ObjectSelector:              commonv1.ObjectSelector{Name: "es", Namespace: "es-ns"},
				ClientCertificateSecretName: "user-provided-cert",
			},
		},
	}
	kbWithoutUserCert := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "kb-no-cert"},
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ElasticsearchSelector{
				ObjectSelector: commonv1.ObjectSelector{Name: "es", Namespace: "es-ns"},
			},
		},
	}

	t.Run("returns only associations with user-provided cert", func(t *testing.T) {
		assocs := []commonv1.Association{kbWithUserCert.EsAssociation(), kbWithoutUserCert.EsAssociation()}
		filtered := filterWithUserProvidedClientCert(assocs)
		require.Len(t, filtered, 1)
		require.Equal(t, "user-provided-cert", filtered[0].AssociationRef().GetClientCertificateSecretName())
	})

	t.Run("returns empty when none have user-provided cert", func(t *testing.T) {
		assocs := []commonv1.Association{kbWithoutUserCert.EsAssociation()}
		filtered := filterWithUserProvidedClientCert(assocs)
		require.Empty(t, filtered)
	})

	t.Run("returns all when all have user-provided cert", func(t *testing.T) {
		assocs := []commonv1.Association{kbWithUserCert.EsAssociation()}
		filtered := filterWithUserProvidedClientCert(assocs)
		require.Len(t, filtered, 1)
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		filtered := filterWithUserProvidedClientCert(nil)
		require.Nil(t, filtered)
	})
}

func TestClientCertSecretName(t *testing.T) {
	kibana := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "my-kibana",
		},
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ElasticsearchSelector{
				ObjectSelector: commonv1.ObjectSelector{Name: "my-es", Namespace: "es-ns"},
			},
		},
	}
	association := kibana.EsAssociation()

	t.Run("generates collision-free secret name with ref hash", func(t *testing.T) {
		secretName := clientCertSecretName(association, "es")
		require.Contains(t, secretName, "my-kibana-es-")
		require.Contains(t, secretName, "-client-cert")
	})

	t.Run("different refs produce different names", func(t *testing.T) {
		kibana2 := kibana.DeepCopy()
		kibana2.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
			ObjectSelector: commonv1.ObjectSelector{Name: "other-es", Namespace: "es-ns"},
		}
		association2 := kibana2.EsAssociation()

		name1 := clientCertSecretName(association, "es")
		name2 := clientCertSecretName(association2, "es")
		require.NotEqual(t, name1, name2, "different refs must produce different secret names")
	})

	t.Run("same ref produces same name", func(t *testing.T) {
		name1 := clientCertSecretName(association, "es")
		name2 := clientCertSecretName(association, "es")
		require.Equal(t, name1, name2)
	})
}

func TestReconciler_Reconcile_ClientCertCreated(t *testing.T) {
	// ES has the client-authentication-required annotation
	esWithClientAuth := sampleES.DeepCopy()
	esWithClientAuth.Annotations = map[string]string{
		annotation.ClientAuthenticationRequiredAnnotation: "true",
	}

	kb := sampleKibanaWithESRef()
	r := testReconciler(&kb, esWithClientAuth, &esHTTPPublicCertsSecret, esHTTPService())

	results, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	// requeue is expected for cert rotation
	require.True(t, results.RequeueAfter > 0, "expected requeue for cert rotation")

	// check that the association conf annotation was set with client cert info
	var updatedKibana kbv1.Kibana
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&kb), &updatedKibana)
	require.NoError(t, err)
	require.Equal(t, commonv1.AssociationEstablished, updatedKibana.Status.AssociationStatus)

	assocConf, err := updatedKibana.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.True(t, assocConf.ClientCertIsConfigured())
	require.NotEmpty(t, assocConf.GetClientCertSecretName())

	// check the client cert secret exists
	var clientCertSecret corev1.Secret
	err = r.Get(context.Background(), types.NamespacedName{
		Namespace: kibanaNamespace,
		Name:      assocConf.GetClientCertSecretName(),
	}, &clientCertSecret)
	require.NoError(t, err)

	// verify secret has tls.crt and tls.key
	require.NotEmpty(t, clientCertSecret.Data[certificates.CertFileName])
	require.NotEmpty(t, clientCertSecret.Data[certificates.KeyFileName])

	// verify self-signed: parse cert and check issuer == subject
	certs, err := certificates.ParsePEMCerts(clientCertSecret.Data[certificates.CertFileName])
	require.NoError(t, err)
	require.Len(t, certs, 1)
	require.Equal(t, certs[0].Subject.CommonName, certs[0].Issuer.CommonName)

	// verify soft-owner labels
	require.Equal(t, "esname", clientCertSecret.Labels[reconciler.SoftOwnerNameLabel])
	require.Equal(t, esNamespace, clientCertSecret.Labels[reconciler.SoftOwnerNamespaceLabel])
	require.Equal(t, esv1.Kind, clientCertSecret.Labels[reconciler.SoftOwnerKindLabel])

	// verify client certificate label
	require.Equal(t, "true", clientCertSecret.Labels[labels.ClientCertificateLabelName])

	// verify association labels for orphan cleanup
	require.Equal(t, "kbname", clientCertSecret.Labels["kibanaassociation.k8s.elastic.co/name"])
	require.Equal(t, kibanaNamespace, clientCertSecret.Labels["kibanaassociation.k8s.elastic.co/namespace"])
}

func TestReconciler_Reconcile_NoClientCertWithoutAnnotation(t *testing.T) {
	// ES does NOT have the client-authentication-required annotation
	kb := sampleKibanaWithESRef()
	r := testReconciler(&kb, &sampleES, &esHTTPPublicCertsSecret, esHTTPService())

	results, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)
	require.Equal(t, reconcile.Result{}, results)

	var updatedKibana kbv1.Kibana
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&kb), &updatedKibana)
	require.NoError(t, err)

	assocConf, err := updatedKibana.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.False(t, assocConf.ClientCertIsConfigured())
	require.Empty(t, assocConf.GetClientCertSecretName())
}

func TestReconciler_Reconcile_ClientCertOrphanCleanup(t *testing.T) {
	// Scenario: Kibana was associated with ES-A (with client auth), now switches to ES-B (without client auth).
	// The orphaned client cert for ES-A should be cleaned up.
	esA := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "es-a",
			Annotations: map[string]string{
				annotation.ClientAuthenticationRequiredAnnotation: "true",
			},
		},
		Status: esv1.ElasticsearchStatus{Version: stackVersion},
	}
	esB := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "es-b",
		},
		Status: esv1.ElasticsearchStatus{Version: stackVersion},
	}

	esAHTTPPublicCerts := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "es-a-es-http-certs-public",
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("ca cert content"),
			"tls.crt": []byte("tls cert content"),
		},
	}
	esBHTTPPublicCerts := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "es-b-es-http-certs-public",
		},
		Data: map[string][]byte{
			"ca.crt":  []byte("ca cert content"),
			"tls.crt": []byte("tls cert content"),
		},
	}
	esAHTTPService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "es-a-es-http",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "https", Port: 9200}},
		},
	}
	esBHTTPService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esNamespace,
			Name:      "es-b-es-http",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "https", Port: 9200}},
		},
	}

	// Step 1: Kibana associated with ES-A (client auth required)
	kbWithEsA := sampleKibanaNoEsRef()
	kbWithEsA.Spec = kbv1.KibanaSpec{
		Version: "7.7.0",
		ElasticsearchRef: commonv1.ElasticsearchSelector{
			ObjectSelector: commonv1.ObjectSelector{Name: esA.Name, Namespace: esA.Namespace},
		},
	}

	r := testReconciler(&kbWithEsA, &esA, &esB, &esAHTTPPublicCerts, &esBHTTPPublicCerts, &esAHTTPService, &esBHTTPService)

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kbWithEsA)})
	require.NoError(t, err)

	// Verify client cert was created for ES-A
	var updatedKb kbv1.Kibana
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&kbWithEsA), &updatedKb)
	require.NoError(t, err)
	assocConf, err := updatedKb.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.True(t, assocConf.ClientCertIsConfigured())
	clientCertSecretName := assocConf.GetClientCertSecretName()
	require.NotEmpty(t, clientCertSecretName)

	// Verify the client cert secret exists
	var clientCertSecret corev1.Secret
	err = r.Get(context.Background(), types.NamespacedName{Namespace: kibanaNamespace, Name: clientCertSecretName}, &clientCertSecret)
	require.NoError(t, err)

	// Step 2: Switch Kibana to reference ES-B (no client auth)
	updatedKb.Spec.ElasticsearchRef = commonv1.ElasticsearchSelector{
		ObjectSelector: commonv1.ObjectSelector{Name: esB.Name, Namespace: esB.Namespace},
	}
	err = r.Client.Update(context.Background(), &updatedKb)
	require.NoError(t, err)

	_, err = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&updatedKb)})
	require.NoError(t, err)

	// The old client cert for ES-A should have been cleaned up (orphan detection)
	err = r.Get(context.Background(), types.NamespacedName{Namespace: kibanaNamespace, Name: clientCertSecretName}, &clientCertSecret)
	require.True(t, apierrors.IsNotFound(err), "orphaned client cert secret should have been deleted")

	// Verify the new association conf does not require client auth
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&updatedKb), &updatedKb)
	require.NoError(t, err)
	newAssocConf, err := updatedKb.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.False(t, newAssocConf.ClientCertIsConfigured())
	require.Empty(t, newAssocConf.GetClientCertSecretName())
}

func TestReconciler_Reconcile_ClientCertRotation(t *testing.T) {
	// Verify that reconciling twice produces the same client cert (idempotent)
	esWithClientAuth := sampleES.DeepCopy()
	esWithClientAuth.Annotations = map[string]string{
		annotation.ClientAuthenticationRequiredAnnotation: "true",
	}

	kb := sampleKibanaWithESRef()
	r := testReconciler(&kb, esWithClientAuth, &esHTTPPublicCertsSecret, esHTTPService())

	// First reconciliation
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)

	var updatedKb kbv1.Kibana
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&kb), &updatedKb)
	require.NoError(t, err)
	assocConf, err := updatedKb.EsAssociation().AssociationConf()
	require.NoError(t, err)

	var firstClientCertSecret corev1.Secret
	err = r.Get(context.Background(), types.NamespacedName{
		Namespace: kibanaNamespace,
		Name:      assocConf.GetClientCertSecretName(),
	}, &firstClientCertSecret)
	require.NoError(t, err)
	firstCertData := firstClientCertSecret.Data[certificates.CertFileName]
	firstKeyData := firstClientCertSecret.Data[certificates.KeyFileName]

	// Second reconciliation (idempotent)
	_, err = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&updatedKb)})
	require.NoError(t, err)

	var secondClientCertSecret corev1.Secret
	err = r.Get(context.Background(), types.NamespacedName{
		Namespace: kibanaNamespace,
		Name:      assocConf.GetClientCertSecretName(),
	}, &secondClientCertSecret)
	require.NoError(t, err)

	// cert and key should be unchanged (reused)
	require.Equal(t, firstCertData, secondClientCertSecret.Data[certificates.CertFileName])
	require.Equal(t, firstKeyData, secondClientCertSecret.Data[certificates.KeyFileName])

	// resourceVersion should be unchanged (no spurious update)
	require.Equal(t, firstClientCertSecret.ResourceVersion, secondClientCertSecret.ResourceVersion,
		"client cert secret should not be updated when nothing changed")
}

func TestReconciler_Reconcile_ClientCertDeletedWhenDisabled(t *testing.T) {
	// ES starts with client auth required
	esWithClientAuth := sampleES.DeepCopy()
	esWithClientAuth.Annotations = map[string]string{
		annotation.ClientAuthenticationRequiredAnnotation: "true",
	}

	kb := sampleKibanaWithESRef()
	r := testReconciler(&kb, esWithClientAuth, &esHTTPPublicCertsSecret, esHTTPService())

	// Reconcile with client auth enabled
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&kb)})
	require.NoError(t, err)

	// Verify client cert was created
	var updatedKb kbv1.Kibana
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&kb), &updatedKb)
	require.NoError(t, err)
	assocConf, err := updatedKb.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.True(t, assocConf.ClientCertIsConfigured())
	clientCertName := assocConf.GetClientCertSecretName()
	require.NotEmpty(t, clientCertName)

	var clientCertSecret corev1.Secret
	err = r.Get(context.Background(), types.NamespacedName{Namespace: kibanaNamespace, Name: clientCertName}, &clientCertSecret)
	require.NoError(t, err)

	// Now disable client auth on ES (remove annotation)
	var currentES esv1.Elasticsearch
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(esWithClientAuth), &currentES)
	require.NoError(t, err)
	delete(currentES.Annotations, annotation.ClientAuthenticationRequiredAnnotation)
	err = r.Client.Update(context.Background(), &currentES)
	require.NoError(t, err)

	// Reconcile again
	_, err = r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&updatedKb)})
	require.NoError(t, err)

	// Client cert secret should be deleted
	err = r.Get(context.Background(), types.NamespacedName{Namespace: kibanaNamespace, Name: clientCertName}, &clientCertSecret)
	require.True(t, apierrors.IsNotFound(err), "client cert secret should be deleted when client auth is disabled")

	// AssociationConf should no longer reference client cert
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&updatedKb), &updatedKb)
	require.NoError(t, err)
	assocConf, err = updatedKb.EsAssociation().AssociationConf()
	require.NoError(t, err)
	require.False(t, assocConf.ClientCertIsConfigured())
	require.Empty(t, assocConf.GetClientCertSecretName())
}
