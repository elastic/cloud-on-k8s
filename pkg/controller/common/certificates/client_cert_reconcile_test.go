// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	"crypto/x509"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func newTestReconciler(c k8s.Client, owner client.Object, lbls map[string]string) Reconciler {
	return Reconciler{
		K8sClient: c,
		Owner:     owner,
		Namer:     esv1.ESNamer,
		Metadata: metadata.Metadata{
			Labels: lbls,
		},
		CertRotation: RotationParams{
			Validity:     DefaultCertValidity,
			RotateBefore: DefaultRotateBefore,
		},
	}
}

func TestReconcileClientCertificate_OperatorCert(t *testing.T) {
	owner := &esv1.Elasticsearch{
		Namespace: "ns",
		Name:      "es",
		UID:       "test-uid",
	}
	secretName := OperatorClientCertSecretName(esv1.ESNamer, owner.Name)

	t.Run("creates new self-signed secret when none exists", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, owner, map[string]string{"app": "test"})

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)
		require.NotNil(t, secret)

		require.Equal(t, secretName, secret.Name)
		require.Equal(t, owner.Namespace, secret.Namespace)

		require.NotEmpty(t, secret.Data[CertFileName])
		require.NotEmpty(t, secret.Data[KeyFileName])
		require.Empty(t, secret.Data[CAFileName])
		require.Equal(t, "test", secret.Labels["app"])

		// No extra labels means no client certificate label
		require.NotContains(t, secret.Labels, labels.ClientCertificateLabelName)

		tlsCert, err := ParseTLSCertificate(secret.Secret)
		require.NoError(t, err)
		require.NotNil(t, tlsCert)

		certs, err := ParsePEMCerts(secret.Data[CertFileName])
		require.NoError(t, err)
		require.Len(t, certs, 1)
		require.Equal(t, internalClientCertCommonName, certs[0].Subject.CommonName)
		require.Equal(t, []string{owner.Name}, certs[0].Subject.OrganizationalUnit)
		require.Contains(t, certs[0].ExtKeyUsage, x509.ExtKeyUsageClientAuth)
		require.NotContains(t, certs[0].ExtKeyUsage, x509.ExtKeyUsageServerAuth)
		require.Equal(t, certs[0].Subject.CommonName, certs[0].Issuer.CommonName)
	})

	t.Run("reuses existing valid secret", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, owner, nil)

		secret1, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)

		secret2, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)

		require.Equal(t, secret1.Data[CertFileName], secret2.Data[CertFileName])
		require.Equal(t, secret1.Data[KeyFileName], secret2.Data[KeyFileName])
	})

	t.Run("generates a new key when renewing expired cert", func(t *testing.T) {
		// Start with a valid cert+key.
		secret := &corev1.Secret{Data: make(map[string][]byte)}
		ctx := context.Background()
		require.NoError(t, ensureClientCertificateSecretContents(ctx, secret, internalClientCertCommonName, owner.Name, RotationParams{
			Validity:     DefaultCertValidity,
			RotateBefore: DefaultRotateBefore,
		}))
		originalKey := secret.Data[KeyFileName]
		existingKey := privateKeyIfValid(ctx, secret)
		require.NotNil(t, existingKey)

		// Replace the cert with an expired one (same key). isClientCertificateValid will return
		// false (cert expired) → renewal is triggered. A fresh key pair must be generated.
		expiredTemplate := createClientCertTemplate(internalClientCertCommonName, owner.Name,
			existingKey.Public(), -time.Hour)
		expiredCertBytes, err := createSelfSignedClientCert(*expiredTemplate, existingKey)
		require.NoError(t, err)
		secret.Data[CertFileName] = EncodePEMCert(expiredCertBytes)

		require.NoError(t, ensureClientCertificateSecretContents(ctx, secret, internalClientCertCommonName, owner.Name, RotationParams{
			Validity:     DefaultCertValidity,
			RotateBefore: DefaultRotateBefore,
		}))

		require.NotEqual(t, originalKey, secret.Data[KeyFileName], "a new private key must be generated on cert renewal")
		require.NotEqual(t, EncodePEMCert(expiredCertBytes), secret.Data[CertFileName], "cert must be renewed")
	})
}

func TestReconcileClientCertificate_WithExtraLabels(t *testing.T) {
	kibana := &kbv1.Kibana{
		Namespace: "ns",
		Name:      "my-kibana",
		UID:       "test-uid",
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{
				Name:      "my-es",
				Namespace: "es-ns",
			}},
		},
	}
	secretName := "my-kibana-es-test-client-cert"

	extraLabels := map[string]string{
		labels.ClientCertificateLabelName: "true",
		"extra":                           "label",
	}

	t.Run("creates secret with the extra labels", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, kibana, map[string]string{"base": "label"})

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, kibana.Name, kibana.Name, extraLabels)
		require.NoError(t, err)
		require.NotNil(t, secret)

		require.Equal(t, secretName, secret.Name)
		require.Equal(t, "true", secret.Labels[labels.ClientCertificateLabelName])
		require.Equal(t, "label", secret.Labels["extra"])
		require.Equal(t, "label", secret.Labels["base"])
	})

	t.Run("derives common name from provided argument", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, kibana, nil)

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, "my-kibana", kibana.Name, extraLabels)
		require.NoError(t, err)

		cert, err := ParseTLSCertificate(secret.Secret)
		require.NoError(t, err)
		require.Equal(t, "my-kibana", cert.Leaf.Subject.CommonName)
	})
}

func TestParseTLSCertificate(t *testing.T) {
	owner := &esv1.Elasticsearch{
		Namespace: "ns", Name: "es", UID: "test-uid",
	}
	c := k8s.NewFakeClient()
	r := newTestReconciler(c, owner, nil)
	selfSignedSecret, err := r.ReconcileClientCertificate(context.Background(), "test-cert", "test", "es", nil)
	require.NoError(t, err)

	certPEM := selfSignedSecret.Data[CertFileName]
	keyPEM := selfSignedSecret.Data[KeyFileName]

	t.Run("parses valid cert and key", func(t *testing.T) {
		secret := corev1.Secret{
			Namespace: "ns", Name: "test",
			Data: map[string][]byte{
				CertFileName: certPEM,
				KeyFileName:  keyPEM,
			},
		}
		tlsCert, err := ParseTLSCertificate(secret)
		require.NoError(t, err)
		require.NotNil(t, tlsCert)
	})

	t.Run("fails on missing cert", func(t *testing.T) {
		secret := corev1.Secret{
			Namespace: "ns", Name: "test",
			Data: map[string][]byte{KeyFileName: keyPEM},
		}
		_, err := ParseTLSCertificate(secret)
		require.Error(t, err)
	})

	t.Run("fails on missing key", func(t *testing.T) {
		secret := corev1.Secret{
			Namespace: "ns", Name: "test",
			Data: map[string][]byte{CertFileName: certPEM},
		}
		_, err := ParseTLSCertificate(secret)
		require.Error(t, err)
	})
}

func TestLoadClientCertIfExists(t *testing.T) {
	owner := &esv1.Elasticsearch{
		Namespace: "ns", Name: "es", UID: "test-uid",
	}

	expectedSecretName := OperatorClientCertSecretName(esv1.ESNamer, owner.Name)

	fakeClient := k8s.NewFakeClient()
	r := newTestReconciler(fakeClient, owner, nil)
	selfSignedSecret, err := r.ReconcileClientCertificate(context.Background(), expectedSecretName, "test", "es", nil)
	require.NoError(t, err)

	t.Run("returns nil when secret does not exist", func(t *testing.T) {
		c := k8s.NewFakeClient()
		tlsCert, err := LoadOperatorClientCertIfExists(context.Background(), c, esv1.ESNamer, "ns", "nonexistent")
		require.NoError(t, err)
		require.Nil(t, tlsCert)
	})

	t.Run("loads existing secret", func(t *testing.T) {
		secret := &corev1.Secret{
			Namespace: "ns", Name: expectedSecretName,
			Data: map[string][]byte{
				CertFileName: selfSignedSecret.Data[CertFileName],
				KeyFileName:  selfSignedSecret.Data[KeyFileName],
			},
		}
		c := k8s.NewFakeClient(secret)

		tlsCert, err := LoadOperatorClientCertIfExists(context.Background(), c, esv1.ESNamer, "ns", owner.Name)
		require.NoError(t, err)
		require.NotNil(t, tlsCert)
	})
}

func TestClientCertSecretCleanup(t *testing.T) {
	owner := &esv1.Elasticsearch{
		Namespace: "ns", Name: "es", UID: "test-uid",
	}
	secretName := OperatorClientCertSecretName(esv1.ESNamer, owner.Name)

	t.Run("secret can be deleted after creation", func(t *testing.T) {
		c := k8s.NewFakeClient()
		r := newTestReconciler(c, owner, nil)

		secret, err := r.ReconcileClientCertificate(context.Background(), secretName, internalClientCertCommonName, owner.Name, nil)
		require.NoError(t, err)

		var fetchedSecret corev1.Secret
		err = c.Get(context.Background(), types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}, &fetchedSecret)
		require.NoError(t, err)

		err = k8s.DeleteSecretIfExists(context.Background(), c, types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name})
		require.NoError(t, err)

		err = c.Get(context.Background(), types.NamespacedName{Namespace: secret.Namespace, Name: secret.Name}, &fetchedSecret)
		require.True(t, apierrors.IsNotFound(err))
	})
}

// newTestServer returns an Elasticsearch standing for the server a trust source is published for.
func newTestServer(namespace, name string) *esv1.Elasticsearch {
	return &esv1.Elasticsearch{Namespace: namespace, Name: name, UID: types.UID(namespace + "-" + name + "-uid")}
}

// newTrustSource returns a secret shaped like a trust source published for the given server. A nil server
// returns a trust source without owner.
func newTrustSource(namespace, name string, server *esv1.Elasticsearch, extraLabels map[string]string) *corev1.Secret {
	secretLabels := map[string]string{commonv1.TypeLabelName: ClientCertTrustSourceType}
	maps.Copy(secretLabels, extraLabels)
	secret := &corev1.Secret{
		Namespace: namespace,
		Name:      name,
		Labels:    secretLabels,
		Annotations: map[string]string{
			clientCertSourceNamespaceAnnotation: "kb-ns",
			clientCertSourceNameAnnotation:      name + "-source",
		},
		Data: map[string][]byte{CertFileName: []byte("cert-data")},
	}
	if server != nil {
		secret.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(server, esv1.GroupVersion.WithKind(esv1.Kind))}
	}
	return secret
}

func TestListClientCertTrustSources(t *testing.T) {
	server := newTestServer("es-ns", "my-es")
	// recreatedServer is a server with the same name and a new UID: it must not claim the trust sources of its
	// predecessor.
	recreatedServer := server.DeepCopy()
	recreatedServer.UID = "recreated-uid"

	// notATrustSource is owned by the server in its namespace, but is not a trust source.
	notATrustSource := newTrustSource("es-ns", "not-a-trust-source", server, nil)
	delete(notATrustSource.Labels, commonv1.TypeLabelName)
	notATrustSource.Labels[labels.ClientCertificateLabelName] = "true"

	for _, tt := range []struct {
		name      string
		server    *esv1.Elasticsearch
		objs      []client.Object
		wantNames []string
	}{
		{
			name:   "lists the trust sources controlled by the server in its namespace",
			server: server,
			objs: []client.Object{
				newTrustSource("es-ns", "source-1", server, nil),
				newTrustSource("es-ns", "source-2", server, nil),
			},
			wantNames: []string{"source-1", "source-2"},
		},
		{
			// A trust source in another namespace is not used: the list is scoped to the server namespace, even
			// if the record is controlled by the server.
			name:   "excludes records in other namespaces",
			server: server,
			objs: []client.Object{
				newTrustSource("es-ns", "source-1", server, nil),
				newTrustSource("kb-ns", "in-other-namespace", server, nil),
			},
			wantNames: []string{"source-1"},
		},
		{
			name:   "excludes records controlled by a different server",
			server: server,
			objs: []client.Object{
				newTrustSource("es-ns", "mine", server, nil),
				newTrustSource("es-ns", "other", newTestServer("es-ns", "other-es"), nil),
			},
			wantNames: []string{"mine"},
		},
		{
			// A record with the trust source label but not controlled by the server was not published by the
			// operator for it.
			name:   "excludes records without owner",
			server: server,
			objs: []client.Object{
				newTrustSource("es-ns", "mine", server, nil),
				newTrustSource("es-ns", "unowned", nil, nil),
			},
			wantNames: []string{"mine"},
		},
		{
			name:   "excludes the trust sources of a previous server with the same name",
			server: recreatedServer,
			objs: []client.Object{
				newTrustSource("es-ns", "stale", server, nil),
				newTrustSource("es-ns", "fresh", recreatedServer, nil),
			},
			wantNames: []string{"fresh"},
		},
		{
			name:   "excludes secrets that are not trust sources",
			server: server,
			objs: []client.Object{
				newTrustSource("es-ns", "source-1", server, nil),
				notATrustSource,
			},
			wantNames: []string{"source-1"},
		},
		{
			name:      "returns empty list when no matching records",
			server:    server,
			wantNames: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objs...)
			sources, err := listClientCertTrustSources(t.Context(), c, tt.server)
			require.NoError(t, err)
			var gotNames []string
			for _, s := range sources {
				gotNames = append(gotNames, s.Name)
			}
			require.Equal(t, tt.wantNames, gotNames)
		})
	}
}

func TestReconcileTrustBundle_OnlyTrustsOwnedTrustSources(t *testing.T) {
	es := newTestServer("es-ns", "my-es")
	// otherKb is an ECK resource in another namespace, which owns the Secret below.
	otherKb := &kbv1.Kibana{Namespace: "kb-ns", Name: "other-kb", UID: "other-kb-uid"}

	// clientCertWithLegacyLabels is a client certificate Secret in another namespace, as created by older versions:
	// it carries server soft-owner labels, the client certificate label and an owner reference to an ECK resource
	// of its own namespace.
	clientCertWithLegacyLabels := &corev1.Secret{
		Namespace: "kb-ns",
		Name:      "legacy-client-cert",
		Labels: map[string]string{
			reconciler.SoftOwnerNameLabel:      es.Name,
			reconciler.SoftOwnerNamespaceLabel: es.Namespace,
			reconciler.SoftOwnerKindLabel:      esv1.Kind,
			labels.ClientCertificateLabelName:  "true",
		},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "kibana.k8s.elastic.co/v1", Kind: "Kibana", Name: otherKb.Name, UID: otherKb.UID,
		}},
		Data: map[string][]byte{CertFileName: []byte("LEGACY-CLIENT-CERT\n")},
	}
	// trustSourceInOtherNamespace is a trust source controlled by the server, but in another namespace.
	trustSourceInOtherNamespace := newTrustSource("kb-ns", "trust-source-in-other-namespace", es, nil)
	trustSourceInOtherNamespace.Data = map[string][]byte{CertFileName: []byte("OTHER-NAMESPACE\n")}
	// unlabelledInServerNS is controlled by the server in its namespace but was not published as a trust source.
	unlabelledInServerNS := newTrustSource("es-ns", "unlabelled", es, nil)
	delete(unlabelledInServerNS.Labels, commonv1.TypeLabelName)
	unlabelledInServerNS.Data = map[string][]byte{CertFileName: []byte("UNLABELLED\n")}
	// unownedInServerNS has the trust source label in the server namespace but is not controlled by the server.
	unownedInServerNS := newTrustSource("es-ns", "unowned", nil, nil)
	unownedInServerNS.Data = map[string][]byte{CertFileName: []byte("UNOWNED\n")}

	ownedTrustSource := newTrustSource("es-ns", "owned-trust-source", es, nil)
	ownedTrustSource.Data = map[string][]byte{CertFileName: []byte("OWNED\n")}
	operatorCert := &CertificatesSecret{
		Namespace: es.Namespace, Name: "operator",
		Data: map[string][]byte{CertFileName: []byte("OPERATOR\n")}}

	for _, tt := range []struct {
		name string
		objs []client.Object
		want string
	}{
		{
			name: "client certificate secret in another namespace is not trusted",
			objs: []client.Object{otherKb, clientCertWithLegacyLabels},
			want: "OPERATOR\n",
		},
		{
			name: "trust source in another namespace is not trusted",
			objs: []client.Object{trustSourceInOtherNamespace},
			want: "OPERATOR\n",
		},
		{
			name: "secret in the server namespace without the trust source label is not trusted",
			objs: []client.Object{unlabelledInServerNS},
			want: "OPERATOR\n",
		},
		{
			name: "trust source in the server namespace not controlled by the server is not trusted",
			objs: []client.Object{unownedInServerNS},
			want: "OPERATOR\n",
		},
		{
			name: "only the trust source controlled by the server in its namespace is trusted",
			objs: []client.Object{otherKb, clientCertWithLegacyLabels, trustSourceInOtherNamespace, unlabelledInServerNS, unownedInServerNS, ownedTrustSource},
			want: "OPERATOR\nOWNED\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(append(tt.objs, es)...)
			r := newTestReconciler(c, es, nil)

			bundle, err := r.ReconcileTrustBundle(t.Context(), operatorCert)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(bundle))

			var trustBundle corev1.Secret
			require.NoError(t, c.Get(t.Context(), types.NamespacedName{
				Namespace: es.Namespace, Name: ClientCertTrustBundleSecretName(r.Namer, es.Name),
			}, &trustBundle))
			require.Equal(t, tt.want, string(trustBundle.Data[ClientCertificatesTrustBundleFileName]))
		})
	}
}

func TestListClientCertTrustSourcesByLabels(t *testing.T) {
	assocLabels := map[string]string{"kibanaassociation.k8s.elastic.co/name": "kb"}
	otherAssocLabels := map[string]string{"kibanaassociation.k8s.elastic.co/name": "other-kb"}
	server1, server2 := newTestServer("es-ns-1", "es-1"), newTestServer("es-ns-2", "es-2")

	// The client certificate secret the trust sources were published from carries the same labels.
	sourceSecret := &corev1.Secret{
		Namespace: "kb-ns",
		Name:      "kb-client-cert",
		Labels:    map[string]string{labels.ClientCertificateLabelName: "true", "kibanaassociation.k8s.elastic.co/name": "kb"},
	}
	c := k8s.NewFakeClient(
		newTrustSource("es-ns-1", "kb-source-1", server1, assocLabels),
		newTrustSource("es-ns-2", "kb-source-2", server2, assocLabels),
		newTrustSource("es-ns-1", "other-kb-source", server1, otherAssocLabels),
		sourceSecret,
	)

	sources, err := ListClientCertTrustSources(t.Context(), c, assocLabels)
	require.NoError(t, err)
	var listed []types.NamespacedName //nolint:prealloc
	for i := range sources {
		listed = append(listed, k8s.ExtractNamespacedName(&sources[i]))
	}
	require.Equal(t, []types.NamespacedName{
		{Namespace: "es-ns-1", Name: "kb-source-1"},
		{Namespace: "es-ns-2", Name: "kb-source-2"},
	}, listed, "only the trust sources carrying the labels are listed, across namespaces")
}

func TestRemoveServerSoftOwnerLabels(t *testing.T) {
	for _, tt := range []struct {
		name       string
		labels     map[string]string
		wantLabels map[string]string
	}{
		{
			name: "removes the server soft-owner labels left by older operator versions",
			labels: map[string]string{
				labels.ClientCertificateLabelName:  "true",
				reconciler.SoftOwnerNameLabel:      "my-es",
				reconciler.SoftOwnerNamespaceLabel: "es-ns",
				reconciler.SoftOwnerKindLabel:      esv1.Kind,
			},
			wantLabels: map[string]string{labels.ClientCertificateLabelName: "true"},
		},
		{
			name:       "leaves a secret without soft-owner labels unchanged",
			labels:     map[string]string{labels.ClientCertificateLabelName: "true"},
			wantLabels: map[string]string{labels.ClientCertificateLabelName: "true"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(&corev1.Secret{Namespace: "kb-ns", Name: "client-cert", Labels: tt.labels})
			var secret corev1.Secret
			require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: "kb-ns", Name: "client-cert"}, &secret))

			require.NoError(t, RemoveServerSoftOwnerLabels(t.Context(), c, &secret))

			var updated corev1.Secret
			require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: "kb-ns", Name: "client-cert"}, &updated))
			require.Equal(t, tt.wantLabels, updated.Labels)
		})
	}
}

func TestReconcileClientCertTrustSource(t *testing.T) {
	server := newTestServer("es-ns", "my-es")
	sourceNSN := types.NamespacedName{Namespace: "kb-ns", Name: "my-kibana-kb-es-client-cert"}
	// meta is the metadata of the publishing resource. It carries the client certificate label of the client
	// certificate secret it was published from, which must not end up on the trust source.
	meta := metadata.Metadata{
		Labels: map[string]string{
			"kibanaassociation.k8s.elastic.co/name": "my-kibana",
			labels.ClientCertificateLabelName:       "true",
		},
		Annotations: map[string]string{"user-annotation": "value"},
	}
	otherSourceNSN := types.NamespacedName{Namespace: "kb-ns", Name: "other-kibana-kb-es-client-cert"}
	publishFrom := func(source types.NamespacedName, meta metadata.Metadata, cert string) func(t *testing.T, c k8s.Client) {
		return func(t *testing.T, c k8s.Client) {
			t.Helper()
			require.NoError(t, ReconcileClientCertTrustSource(t.Context(), c, server, source, []byte(cert), meta))
		}
	}
	publish := func(cert string) func(t *testing.T, c k8s.Client) {
		return publishFrom(sourceNSN, meta, cert)
	}
	// previousServer has the name of the server but not its UID: it was deleted and recreated.
	previousServer := server.DeepCopy()
	previousServer.UID = "previous-uid"
	publishAs := func(owner *esv1.Elasticsearch, cert string) func(t *testing.T, c k8s.Client) {
		return func(t *testing.T, c k8s.Client) {
			t.Helper()
			require.NoError(t, ReconcileClientCertTrustSource(t.Context(), c, owner, sourceNSN, []byte(cert), meta))
		}
	}
	deleteSource := func(source types.NamespacedName) func(t *testing.T, c k8s.Client) {
		return func(t *testing.T, c k8s.Client) {
			t.Helper()
			require.NoError(t, DeleteClientCertTrustSource(t.Context(), c, server.Namespace, source))
		}
	}
	// expectedTrustSource returns the trust source expected to be published for the given source, metadata
	// labels and annotations and certificate.
	expectedTrustSourceFrom := func(source types.NamespacedName, extraLabels, extraAnnotations map[string]string, cert string) corev1.Secret {
		return corev1.Secret{
			Namespace: server.Namespace,
			Name:      clientCertTrustSourceSecretName(source.Namespace, source.Name),
			Labels:    maps.Clone(extraLabels),
			Annotations: map[string]string{
				clientCertSourceNamespaceAnnotation: source.Namespace,
				clientCertSourceNameAnnotation:      source.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(server, esv1.GroupVersion.WithKind(esv1.Kind))},
			Data:            map[string][]byte{CertFileName: []byte(cert)},
		}
	}
	expectedTrustSource := func(cert string) corev1.Secret {
		secret := expectedTrustSourceFrom(sourceNSN,
			map[string]string{
				"kibanaassociation.k8s.elastic.co/name":    "my-kibana",
				commonv1.TypeLabelName:                     ClientCertTrustSourceType,
				commonv1.RestrictWatchedResourcesLabelName: commonv1.RestrictWatchedResourcesLabelValue,
			}, nil, cert)
		secret.Annotations["user-annotation"] = "value"
		return secret
	}
	emptyMetaTrustSource := expectedTrustSourceFrom(otherSourceNSN,
		map[string]string{
			commonv1.TypeLabelName:                     ClientCertTrustSourceType,
			commonv1.RestrictWatchedResourcesLabelName: commonv1.RestrictWatchedResourcesLabelValue,
		}, nil, "other-cert")

	for _, tt := range []struct {
		name        string
		steps       []func(t *testing.T, c k8s.Client)
		wantSources []corev1.Secret
	}{
		{
			name:        "publishes the certificate into the server namespace, owned by the server",
			steps:       []func(t *testing.T, c k8s.Client){publish("public-cert")},
			wantSources: []corev1.Secret{expectedTrustSource("public-cert")},
		},
		{
			name:        "updates the certificate when it is published again",
			steps:       []func(t *testing.T, c k8s.Client){publish("public-cert"), publish("rotated-cert")},
			wantSources: []corev1.Secret{expectedTrustSource("rotated-cert")},
		},
		{
			name: "deletes the trust source",
			steps: []func(t *testing.T, c k8s.Client){
				publish("public-cert"),
				func(t *testing.T, c k8s.Client) {
					t.Helper()
					require.NoError(t, DeleteClientCertTrustSource(t.Context(), c, server.Namespace, sourceNSN))
				},
			},
			wantSources: nil,
		},
		{
			// The trust source of the previous server may not be garbage collected yet: it must not be left
			// controlled by the previous server, otherwise the server would ignore it.
			name: "a trust source of a previous server with the same name is owned by the current server",
			steps: []func(t *testing.T, c k8s.Client){
				publishAs(previousServer, "public-cert"),
				publishAs(server, "public-cert"),
			},
			wantSources: []corev1.Secret{expectedTrustSource("public-cert")},
		},
		{
			name:        "deleting a trust source that does not exist is a no-op",
			steps:       []func(t *testing.T, c k8s.Client){deleteSource(sourceNSN)},
			wantSources: nil,
		},
		{
			name: "deleting a trust source keeps the trust sources of the other clients",
			steps: []func(t *testing.T, c k8s.Client){
				publish("public-cert"),
				publishFrom(otherSourceNSN, metadata.Metadata{}, "other-cert"),
				deleteSource(sourceNSN),
			},
			wantSources: []corev1.Secret{emptyMetaTrustSource},
		},
		{
			name:        "publishes a trust source for empty metadata",
			steps:       []func(t *testing.T, c k8s.Client){publishFrom(otherSourceNSN, metadata.Metadata{}, "other-cert")},
			wantSources: []corev1.Secret{emptyMetaTrustSource},
		},
		{
			// Two clients published under the same name from different namespaces must not overwrite each other.
			name: "keeps trust sources of clients with the same name in different namespaces",
			steps: []func(t *testing.T, c k8s.Client){
				publishFrom(types.NamespacedName{Namespace: "kb-a", Name: "kb-client-cert"}, metadata.Metadata{}, "cert-a"),
				publishFrom(types.NamespacedName{Namespace: "kb-b", Name: "kb-client-cert"}, metadata.Metadata{}, "cert-b"),
			},
			wantSources: []corev1.Secret{
				expectedTrustSourceFrom(types.NamespacedName{Namespace: "kb-a", Name: "kb-client-cert"},
					map[string]string{
						commonv1.TypeLabelName:                     ClientCertTrustSourceType,
						commonv1.RestrictWatchedResourcesLabelName: commonv1.RestrictWatchedResourcesLabelValue,
					}, nil, "cert-a"),
				expectedTrustSourceFrom(types.NamespacedName{Namespace: "kb-b", Name: "kb-client-cert"},
					map[string]string{
						commonv1.TypeLabelName:                     ClientCertTrustSourceType,
						commonv1.RestrictWatchedResourcesLabelName: commonv1.RestrictWatchedResourcesLabelValue,
					}, nil, "cert-b"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient()
			for _, step := range tt.steps {
				step(t, c)
			}

			sources, err := listClientCertTrustSources(t.Context(), c, server)
			require.NoError(t, err)
			// Compare only the fields set by ReconcileClientCertTrustSource, not the ones set by the API server.
			var gotSources []corev1.Secret //nolint:prealloc
			for _, s := range sources {
				gotSources = append(gotSources, corev1.Secret{
					Namespace: s.Namespace, Name: s.Name, Labels: s.Labels, Annotations: s.Annotations,
					OwnerReferences: s.OwnerReferences, Data: s.Data,
				})
			}
			require.ElementsMatch(t, tt.wantSources, gotSources)
		})
	}
}

func TestClientCertTrustSourceOrigin(t *testing.T) {
	for _, tt := range []struct {
		name        string
		annotations map[string]string
		want        types.NamespacedName
		wantOK      bool
	}{
		{
			name: "returns the origin recorded on the trust source",
			annotations: map[string]string{
				clientCertSourceNamespaceAnnotation: "kb-ns",
				clientCertSourceNameAnnotation:      "kb-client-cert",
			},
			want:   types.NamespacedName{Namespace: "kb-ns", Name: "kb-client-cert"},
			wantOK: true,
		},
		{
			name:        "is not a trust source without annotations",
			annotations: nil,
		},
		{
			name:        "is not a trust source without the origin namespace",
			annotations: map[string]string{clientCertSourceNameAnnotation: "kb-client-cert"},
		},
		{
			name:        "is not a trust source without the origin name",
			annotations: map[string]string{clientCertSourceNamespaceAnnotation: "kb-ns"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ClientCertTrustSourceOrigin(corev1.Secret{Annotations: tt.annotations})
			require.Equal(t, tt.wantOK, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestClientCertTrustSources_ClientErrors(t *testing.T) {
	server := newTestServer("es-ns", "my-es")
	sourceNSN := types.NamespacedName{Namespace: "kb-ns", Name: "kb-client-cert"}
	failSecrets := interceptor.Funcs{
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return fmt.Errorf("simulated create failure")
		},
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return fmt.Errorf("simulated list failure")
		},
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return fmt.Errorf("simulated patch failure")
		},
	}
	clientCertSecret := &corev1.Secret{
		Namespace: "kb-ns", Name: "kb-client-cert",
		Labels: map[string]string{reconciler.SoftOwnerNameLabel: "my-es"},
	}

	for _, tt := range []struct {
		name string
		objs []client.Object
		// call runs the function under test with a client failing every create, list and patch.
		call    func(t *testing.T, c k8s.Client) error
		wantErr string
		// check runs on the failing client's underlying state, once the call returned.
		check func(t *testing.T, c k8s.Client)
	}{
		{
			name: "publishing a trust source fails when the secret cannot be created",
			call: func(t *testing.T, c k8s.Client) error {
				t.Helper()
				return ReconcileClientCertTrustSource(t.Context(), c, server, sourceNSN, []byte("cert"), metadata.Metadata{})
			},
			wantErr: "simulated create failure",
		},
		{
			name: "listing trust sources by labels fails when the secrets cannot be listed",
			call: func(t *testing.T, c k8s.Client) error {
				t.Helper()
				_, err := ListClientCertTrustSources(t.Context(), c, nil)
				return err
			},
			wantErr: "failed to list client certificate trust sources",
		},
		{
			name: "the trust bundle is not written when the trust sources cannot be listed",
			call: func(t *testing.T, c k8s.Client) error {
				t.Helper()
				r := newTestReconciler(c, server, nil)
				_, err := r.ReconcileTrustBundle(t.Context())
				return err
			},
			wantErr: "failed to list client certificate trust sources",
			check: func(t *testing.T, c k8s.Client) {
				t.Helper()
				var bundle corev1.Secret
				err := c.Get(t.Context(), types.NamespacedName{
					Namespace: server.Namespace, Name: ClientCertTrustBundleSecretName(esv1.ESNamer, server.Name),
				}, &bundle)
				require.True(t, apierrors.IsNotFound(err), "the trust bundle must not be written")
			},
		},
		{
			name: "removing the server soft-owner labels fails when the secret cannot be patched",
			objs: []client.Object{clientCertSecret},
			call: func(t *testing.T, c k8s.Client) error {
				t.Helper()
				var secret corev1.Secret
				require.NoError(t, c.Get(t.Context(), k8s.ExtractNamespacedName(clientCertSecret), &secret))
				return RemoveServerSoftOwnerLabels(t.Context(), c, &secret)
			},
			wantErr: "simulated patch failure",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClientBuilder(tt.objs...).WithInterceptorFuncs(failSecrets).Build()

			err := tt.call(t, c)
			require.ErrorContains(t, err, tt.wantErr)
			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

func TestBuildTrustBundleFromSecrets(t *testing.T) {
	t.Run("concatenates tls.crt from secrets", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				Namespace: "ns1", Name: "secret1",
				Data: map[string][]byte{CertFileName: []byte("CERT-1\n")},
			},
			{
				Namespace: "ns2", Name: "secret2",
				Data: map[string][]byte{CertFileName: []byte("CERT-2\n")},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "CERT-1\nCERT-2\n", string(bundle))
	})

	t.Run("skips secrets without tls.crt", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				Namespace: "ns1", Name: "secret1",
				Data: map[string][]byte{CertFileName: []byte("CERT\n")},
			},
			{
				Namespace: "ns2", Name: "secret2",
				Data: map[string][]byte{},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "CERT\n", string(bundle))
	})

	t.Run("sorts secrets by namespace/name for deterministic output", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				Namespace: "ns2", Name: "secret-b",
				Data: map[string][]byte{CertFileName: []byte("B")},
			},
			{
				Namespace: "ns1", Name: "secret-a",
				Data: map[string][]byte{CertFileName: []byte("A")},
			},
			{
				Namespace: "ns2", Name: "secret-a",
				Data: map[string][]byte{CertFileName: []byte("C")},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "A\nC\nB\n", string(bundle))
	})

	t.Run("sorts trust sources by originating secret, mixed with directly passed secrets", func(t *testing.T) {
		// Trust source names are hashed, so sorting by them would not preserve the order of the originating
		// client certificate secrets. The operator certificate has no origin and sorts by its own identity.
		trustSourceFor := func(name, origNamespace, origName, cert string) corev1.Secret {
			return corev1.Secret{
				Namespace: "es-ns", Name: name,
				Annotations: map[string]string{
					clientCertSourceNamespaceAnnotation: origNamespace,
					clientCertSourceNameAnnotation:      origName,
				},
				Data: map[string][]byte{CertFileName: []byte(cert)},
			}
		}
		secrets := []corev1.Secret{
			trustSourceFor("aaa-client-cert-source", "kb-b", "kb-client-cert", "KB-B"),
			{
				Namespace: "es-ns", Name: "es-internal-client-cert",
				Data: map[string][]byte{CertFileName: []byte("OPERATOR")},
			},
			trustSourceFor("zzz-client-cert-source", "kb-a", "kb-client-cert", "KB-A"),
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "OPERATOR\nKB-A\nKB-B\n", string(bundle))
	})

	t.Run("does not double newline when cert already ends with newline", func(t *testing.T) {
		secrets := []corev1.Secret{
			{
				Namespace: "ns1", Name: "a",
				Data: map[string][]byte{CertFileName: []byte("CERT-1\n")},
			},
			{
				Namespace: "ns1", Name: "b",
				Data: map[string][]byte{CertFileName: []byte("CERT-2")},
			},
		}

		bundle := buildTrustBundleFromSecrets(context.Background(), secrets)
		require.Equal(t, "CERT-1\nCERT-2\n", string(bundle))
	})
}

func TestDeleteClientCertResources(t *testing.T) {
	esName := "my-es"
	esNS := "my-ns"

	operatorCertSecret := &corev1.Secret{
		Namespace: esNS, Name: OperatorClientCertSecretName(esv1.ESNamer, esName),
	}
	trustBundleSecret := &corev1.Secret{
		Namespace: esNS, Name: ClientCertTrustBundleSecretName(esv1.ESNamer, esName),
	}
	owner := newTestServer(esNS, esName)
	ownedTrustSource1 := newTrustSource(esNS, "owned-trust-source-1", owner, nil)
	ownedTrustSource2 := newTrustSource(esNS, "owned-trust-source-2", owner, nil)
	unownedTrustSource := newTrustSource(esNS, "unowned-trust-source", nil, nil)
	otherServerTrustSource := newTrustSource(esNS, "other-server-trust-source", newTestServer(esNS, "other-es"), nil)
	otherNamespaceTrustSource := newTrustSource("other-ns", "other-namespace-trust-source", owner, nil)

	tests := []struct {
		name                  string
		extraInitialObjs      []client.Object
		interceptorFuncs      *interceptor.Funcs
		wantErr               bool
		wantAnnotationPresent bool
		// wantTrustSources are the names of the trust sources remaining in the cluster.
		wantTrustSources []string
	}{
		{
			name:             "deletes annotation, operator cert secret, and trust bundle secret",
			extraInitialObjs: []client.Object{operatorCertSecret, trustBundleSecret},
		},
		{
			name:             "deletes the trust sources controlled by the server",
			extraInitialObjs: []client.Object{operatorCertSecret, trustBundleSecret, ownedTrustSource1, ownedTrustSource2},
		},
		{
			name:             "keeps the trust sources which are not controlled by the server in its namespace",
			extraInitialObjs: []client.Object{ownedTrustSource1, unownedTrustSource, otherServerTrustSource, otherNamespaceTrustSource},
			wantTrustSources: []string{unownedTrustSource.Name, otherServerTrustSource.Name, otherNamespaceTrustSource.Name},
		},
		{
			name:             "annotation removed even when trust source deletion fails",
			extraInitialObjs: []client.Object{ownedTrustSource1},
			interceptorFuncs: &interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == ownedTrustSource1.GetName() {
						return fmt.Errorf("simulated delete failure")
					}
					return c.Delete(ctx, obj, opts...)
				},
			},
			wantErr:               true,
			wantAnnotationPresent: false,
			wantTrustSources:      []string{ownedTrustSource1.Name},
		},
		{
			name: "no-op when resources already absent",
		},
		{
			name:             "annotation removed even when operator cert secret deletion fails",
			extraInitialObjs: []client.Object{operatorCertSecret, trustBundleSecret},
			interceptorFuncs: &interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == operatorCertSecret.GetName() {
						return fmt.Errorf("simulated delete failure")
					}
					return c.Delete(ctx, obj, opts...)
				},
			},
			wantErr:               true,
			wantAnnotationPresent: false,
		},
		{
			name:             "annotation removed even when trust bundle secret deletion fails",
			extraInitialObjs: []client.Object{operatorCertSecret, trustBundleSecret},
			interceptorFuncs: &interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == trustBundleSecret.GetName() {
						return fmt.Errorf("simulated delete failure")
					}
					return c.Delete(ctx, obj, opts...)
				},
			},
			wantErr:               true,
			wantAnnotationPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := owner.DeepCopy()
			es.Annotations = map[string]string{
				annotation.ClientAuthenticationRequiredAnnotation: "true",
			}

			ctx := context.Background()
			builder := k8s.NewFakeClientBuilder(slices.Concat([]client.Object{es}, tt.extraInitialObjs)...)
			if tt.interceptorFuncs != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptorFuncs)
			}
			c := builder.Build()

			err := DeleteClientCertResources(ctx, c, es, esv1.ESNamer)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			var updatedES esv1.Elasticsearch
			require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: esNS, Name: esName}, &updatedES))
			require.Equal(t, tt.wantAnnotationPresent, annotation.HasClientAuthenticationRequired(&updatedES))

			trustSources, err := ListClientCertTrustSources(ctx, c, nil)
			require.NoError(t, err)
			var gotTrustSources []string
			for _, ts := range trustSources {
				gotTrustSources = append(gotTrustSources, ts.Name)
			}
			require.ElementsMatch(t, tt.wantTrustSources, gotTrustSources)
		})
	}
}
