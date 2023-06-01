// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	testPemPrivateKey = `
-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQCxoeCUW5KJxNPxMp+KmCxKLc1Zv9Ny+4CFqcUXVUYH69L3mQ7v
IWrJ9GBfcaA7BPQqUlWxWM+OCEQZH1EZNIuqRMNQVuIGCbz5UQ8w6tS0gcgdeGX7
J7jgCQ4RK3F/PuCM38QBLaHx988qG8NMc6VKErBjctCXFHQt14lerd5KpQIDAQAB
AoGAYrf6Hbk+mT5AI33k2Jt1kcweodBP7UkExkPxeuQzRVe0KVJw0EkcFhywKpr1
V5eLMrILWcJnpyHE5slWwtFHBG6a5fLaNtsBBtcAIfqTQ0Vfj5c6SzVaJv0Z5rOd
7gQF6isy3t3w9IF3We9wXQKzT6q5ypPGdm6fciKQ8RnzREkCQQDZwppKATqQ41/R
vhSj90fFifrGE6aVKC1hgSpxGQa4oIdsYYHwMzyhBmWW9Xv/R+fPyr8ZwPxp2c12
33QwOLPLAkEA0NNUb+z4ebVVHyvSwF5jhfJxigim+s49KuzJ1+A2RaSApGyBZiwS
rWvWkB471POAKUYt5ykIWVZ83zcceQiNTwJBAMJUFQZX5GDqWFc/zwGoKkeR49Yi
MTXIvf7Wmv6E++eFcnT461FlGAUHRV+bQQXGsItR/opIG7mGogIkVXa3E1MCQARX
AAA7eoZ9AEHflUeuLn9QJI/r0hyQQLEtrpwv6rDT1GCWaLII5HJ6NUFVf4TTcqxo
6vdM4QGKTJoO+SaCyP0CQFdpcxSAuzpFcKv0IlJ8XzS/cy+mweCMwyJ1PFEc4FX6
wg/HcAJWY60xZTJDFN+Qfx8ZQvBEin6c2/h+zZi5IVY=
-----END RSA PRIVATE KEY-----
`
)

// fixtures
var (
	testCA            *CA
	testRSAPrivateKey *rsa.PrivateKey
	cert, pemTLS      []byte
	testES            = esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"}}
	testSvc           = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "2.2.3.3",
		},
	}
)

func init() {
	var err error
	block, _ := pem.Decode([]byte(testPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse private key: " + err.Error())
	}

	if testCA, err = NewSelfSignedCA(CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-common-name"},
		PrivateKey: testRSAPrivateKey,
	}); err != nil {
		panic("Failed to create new self signed CA: " + err.Error())
	}

	testCSRBytes, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, testRSAPrivateKey)
	if err != nil {
		panic("Failed to create CSR:" + err.Error())
	}
	testCSR, _ := x509.ParseCertificateRequest(testCSRBytes)

	validatedCertificateTemplate := createValidatedHTTPCertificateTemplate(
		k8s.ExtractNamespacedName(&testES),
		esv1.ESNamer,
		testES.Spec.HTTP.TLS,
		[]commonv1.SubjectAlternativeName{},
		[]corev1.Service{testSvc},
		testCSR,
		DefaultCertValidity,
	)

	certData, err := testCA.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		panic("Failed to create cert data:" + err.Error())
	}
	cert = certData
	// pemCert contains the certificate and the CA certificate
	pemTLS = EncodePEMCert(certData, testCA.Cert.Raw)
}

func TestReconcilePublicHTTPCerts(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")

	owner := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
		TypeMeta:   metav1.TypeMeta{Kind: esv1.Kind},
	}

	certificate := &CertificatesSecret{
		Secret: corev1.Secret{
			Data: map[string][]byte{
				CAFileName:   ca,
				CertFileName: tls,
				KeyFileName:  key,
			},
		},
	}

	namespacedSecretName := PublicCertsSecretRef(esv1.ESNamer, k8s.ExtractNamespacedName(owner))

	mkClient := func(t *testing.T, objs ...client.Object) k8s.Client {
		t.Helper()
		return k8s.NewFakeClient(objs...)
	}

	labels := map[string]string{
		"expected":                         "default-labels",
		reconciler.SoftOwnerKindLabel:      owner.Kind,
		reconciler.SoftOwnerNamespaceLabel: owner.Namespace,
		reconciler.SoftOwnerNameLabel:      owner.Name,
	}

	mkWantedSecret := func(t *testing.T) *corev1.Secret {
		t.Helper()
		wantSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespacedSecretName.Namespace,
				Name:      namespacedSecretName.Name,
				Labels:    labels,
			},
			Data: map[string][]byte{
				CertFileName: tls,
				CAFileName:   ca,
			},
		}

		return wantSecret
	}

	tests := []struct {
		name       string
		client     func(*testing.T, ...client.Object) k8s.Client
		wantSecret func(*testing.T) *corev1.Secret
		wantErr    bool
	}{
		{
			name:       "is created if missing",
			client:     mkClient,
			wantSecret: mkWantedSecret,
		},
		{
			name: "is updated on mismatch",
			client: func(t *testing.T, _ ...client.Object) k8s.Client {
				t.Helper()
				s := mkWantedSecret(t)
				s.Data[CertFileName] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "removes extraneous keys",
			client: func(t *testing.T, _ ...client.Object) k8s.Client {
				t.Helper()
				s := mkWantedSecret(t)
				s.Data["extra"] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "preserves labels and annotations",
			client: func(t *testing.T, _ ...client.Object) k8s.Client {
				t.Helper()
				s := mkWantedSecret(t)
				s.Labels["label1"] = "labelValue1"
				s.Labels["label2"] = "labelValue2"
				if s.Annotations == nil {
					s.Annotations = make(map[string]string)
				}
				s.Annotations["annotation1"] = "annotationValue1"
				s.Annotations["annotation2"] = "annotationValue2"
				return mkClient(t, s)
			},
			wantSecret: func(t *testing.T) *corev1.Secret {
				t.Helper()
				s := mkWantedSecret(t)
				s.Labels["label1"] = "labelValue1"
				s.Labels["label2"] = "labelValue2"
				if s.Annotations == nil {
					s.Annotations = make(map[string]string)
				}
				s.Annotations["annotation1"] = "annotationValue1"
				s.Annotations["annotation2"] = "annotationValue2"
				return s
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client(t)
			err := Reconciler{
				K8sClient: client,
				Owner:     owner,
				Namer:     esv1.ESNamer,
				Labels:    labels,
			}.ReconcilePublicHTTPCerts(context.Background(), certificate)
			if tt.wantErr {
				require.Error(t, err, "Failed to reconcile")
				return
			}

			var gotSecret corev1.Secret
			err = client.Get(context.Background(), namespacedSecretName, &gotSecret)
			require.NoError(t, err, "Failed to get secret")

			wantSecret := tt.wantSecret(t)
			comparison.AssertEqual(t, wantSecret, &gotSecret)
		})
	}
}

func TestReconcileInternalHTTPCerts(t *testing.T) {
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")
	testCA2, err := NewSelfSignedCA(CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-common-name"},
		PrivateKey: testRSAPrivateKey,
	})
	assert.NoError(t, err, "Failed to create new self signed CA")
	testPrivateKey, err := EncodePEMPrivateKey(testRSAPrivateKey)
	assert.NoError(t, err, "Failed to encode private key")

	customCertFixture := CertificatesSecret{
		Secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cert", Namespace: "test-namespace"},
			Data: map[string][]byte{
				CertFileName: tls,
				KeyFileName:  key,
			},
		},
	}
	type args struct {
		es                          esv1.Elasticsearch
		ca                          *CA
		custCerts                   *CertificatesSecret
		disableInternalCADefaulting bool
		services                    []corev1.Service
		initialObjects              []client.Object
	}
	tests := []struct {
		name    string
		args    args
		want    func(t *testing.T, c k8s.Client, cs *CertificatesSecret)
		wantErr bool
	}{
		{
			name: "should update CA in es-http-certs-public",
			args: args{
				initialObjects: []client.Object{
					// es-http-ca-internal uses a new CA
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testES.Name + "-es-http-ca-internal",
							Namespace: testES.Namespace,
						},
						Data: map[string][]byte{
							"tls.key": testPrivateKey,
							"tls.crt": EncodePEMCert(testCA2.Cert.Raw), // new CA
						},
					},
					// es-http-certs-internal holds the old CA
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testES.Name + "-es-http-certs-internal",
							Namespace: testES.Namespace,
						},
						Data: map[string][]byte{
							"tls.key": testPrivateKey,
							"tls.crt": pemTLS, // PEM TLS with the OLD CA
						},
					},
				},
				es:       testES,
				ca:       testCA2, // es-http-certs-internal should be updated with the new CA
				services: []corev1.Service{testSvc},
			},
			want: func(t *testing.T, c k8s.Client, cs *CertificatesSecret) {
				t.Helper()
				assert.NotNil(t, cs)
				if cs != nil {
					assert.Equal(t, testPrivateKey, cs.Data["tls.key"], "Private key should not have been updated")
					assert.Equal(t, EncodePEMCert(cert, testCA2.Cert.Raw), cs.Data["tls.crt"], "Unexpected tls.crt content in *-es-http-certs-public")
					assert.Equal(t, EncodePEMCert(testCA2.Cert.Raw), cs.Data["ca.crt"], "Unexpected CA certificate in *-es-http-certs-public")
				}
			},
		},
		{
			name: "should generate new certificates if none exists",
			args: args{
				es: testES,
				ca: testCA,
			},
			want: func(t *testing.T, c k8s.Client, cs *CertificatesSecret) {
				t.Helper()
				assert.Contains(t, cs.Data, KeyFileName)
				assert.Contains(t, cs.Data, CertFileName)
			},
		},
		{
			name: "should NOT return a CA if none has been provided by the user",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
					Spec: esv1.ElasticsearchSpec{
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								Certificate: commonv1.SecretRef{
									SecretName: "my-cert",
								},
							},
						},
					},
				},
				ca:        testCA,
				custCerts: &customCertFixture,
			},
			want: func(t *testing.T, c k8s.Client, cs *CertificatesSecret) {
				t.Helper()
				assert.Equal(t, cs.Data[KeyFileName], key)
				assert.Equal(t, cs.Data[CertFileName], tls)

				// We do not expect the CA to be present in the result since none has been provided by the user
				_, hasCaCert := cs.Data[CAFileName]
				assert.False(t, hasCaCert)

				// Retrieve the Secret that contains the data for the internal HTTP certificate
				internalSecret := &corev1.Secret{}
				assert.NoError(t, c.Get(context.Background(), k8s.ExtractNamespacedName(cs), internalSecret))
				// We are still expecting a CA cert to exist in this Secret
				assert.True(t, len(internalSecret.Data[CAFileName]) > 0)
				assert.Equal(t, internalSecret.Data[CAFileName], EncodePEMCert(testCA.Cert.Raw))
			},
		},
		{
			name: "should NOT default to internal CA if so requested",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
					Spec: esv1.ElasticsearchSpec{
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								Certificate: commonv1.SecretRef{
									SecretName: "my-cert",
								},
							},
						},
					},
				},
				ca:                          testCA,
				disableInternalCADefaulting: true,
				custCerts:                   &customCertFixture,
			},
			want: func(t *testing.T, c k8s.Client, cs *CertificatesSecret) {
				t.Helper()
				assert.Equal(t, cs.Data[KeyFileName], key)
				assert.Equal(t, cs.Data[CertFileName], tls)

				// We do not expect the CA to be present in the result since none has been provided by the user
				_, hasCaCert := cs.Data[CAFileName]
				assert.False(t, hasCaCert, "No CA cert in certificates secret struct expected")

				// Retrieve the Secret that contains the data for the internal HTTP certificate
				internalSecret := &corev1.Secret{}
				assert.NoError(t, c.Get(context.Background(), k8s.ExtractNamespacedName(cs), internalSecret))
				// We are also not expecting a CA cert to exist in this internal Secret
				assert.Empty(t, internalSecret.Data[CAFileName], "no CA in internal secret expected")
			},
		},
		{
			name: "should return an unknown private CA provided by the user",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
					Spec: esv1.ElasticsearchSpec{
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								Certificate: commonv1.SecretRef{
									SecretName: "my-cert",
								},
							},
						},
					},
				},
				ca: testCA,
				custCerts: &CertificatesSecret{
					Secret: corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "my-cert", Namespace: "test-namespace"},
						Data: map[string][]byte{
							CAFileName:   EncodePEMCert(testCA.Cert.Raw),
							CertFileName: tls,
							KeyFileName:  key,
						},
					},
				},
			},
			want: func(t *testing.T, c k8s.Client, cs *CertificatesSecret) {
				t.Helper()
				assert.Equal(t, cs.Data[CAFileName], EncodePEMCert(testCA.Cert.Raw))
				assert.Equal(t, cs.Data[KeyFileName], key)
				assert.Equal(t, cs.Data[CertFileName], tls)

				// Retrieve the Secret that contains the data for the internal HTTP certificate
				internalSecret := &corev1.Secret{}
				assert.NoError(t, c.Get(context.Background(), k8s.ExtractNamespacedName(cs), internalSecret))
				assert.True(t, len(internalSecret.Data[CAFileName]) > 0)
				// We expect the private, unknown, CA to be in the result
				assert.Equal(t, internalSecret.Data[CAFileName], EncodePEMCert(testCA.Cert.Raw))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := watches.NewDynamicWatches()
			c := k8s.NewFakeClient(tt.args.initialObjects...)
			got, err := Reconciler{
				K8sClient:      c,
				DynamicWatches: w,
				Owner:          &tt.args.es,
				TLSOptions:     tt.args.es.Spec.HTTP.TLS,
				Namer:          esv1.ESNamer,
				Labels:         map[string]string{},
				Services:       tt.args.services,
				CertRotation: RotationParams{
					Validity:     DefaultCertValidity,
					RotateBefore: DefaultRotateBefore,
				},
				DisableInternalCADefaulting: tt.args.disableInternalCADefaulting,
			}.ReconcileInternalHTTPCerts(context.Background(), tt.args.ca, tt.args.custCerts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileInternalHTTPCerts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.want(t, c, got)
		})
	}
}

func Test_createValidatedHTTPCertificateTemplate(t *testing.T) {
	sanDNS1 := "my.dns.com"
	sanDNS2 := "my.second.dns.com"
	sanIP1 := "4.4.6.7"
	sanIPv6 := "2001:db8:0:85a3:0:0:ac1f:8001"

	type args struct {
		es            esv1.Elasticsearch
		svcs          []corev1.Service
		extraHTTPSANs []commonv1.SubjectAlternativeName
		certValidity  time.Duration
	}
	tests := []struct {
		name string
		args args
		want func(t *testing.T, cert *ValidatedCertificateTemplate)
	}{
		{
			name: "with svcs and user-provided SANs",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "test"},
					Spec: esv1.ElasticsearchSpec{
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
										{
											DNS: sanDNS1,
										},
										{
											DNS: sanDNS2,
										},
										{
											IP: sanIP1,
										},
										{
											IP: sanIPv6,
										},
									},
								},
							},
						},
					},
				},
				svcs: []corev1.Service{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "svc-namespace",
							Name:      "svc-name",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.11.12.13",
						},
					},
				},
				extraHTTPSANs: []commonv1.SubjectAlternativeName{
					{
						DNS: "controller-san-1",
					},
					{
						DNS: "controller-san-2",
					},
				},
			},
			want: func(t *testing.T, cert *ValidatedCertificateTemplate) {
				t.Helper()
				expectedCommonName := "test-es-http.test.es.local"
				assert.Contains(t, cert.Subject.CommonName, expectedCommonName)
				assert.Contains(t, cert.DNSNames, expectedCommonName)
				assert.Contains(t, cert.DNSNames, "svc-name.svc-namespace.svc")
				assert.Contains(t, cert.DNSNames, sanDNS1)
				assert.Contains(t, cert.DNSNames, sanDNS2)
				assert.Contains(t, cert.DNSNames, "controller-san-1")
				assert.Contains(t, cert.DNSNames, "controller-san-2")
				assert.Contains(t, cert.IPAddresses, net.ParseIP(sanIP1).To4())
				assert.Contains(t, cert.IPAddresses, net.ParseIP(sanIPv6))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createValidatedHTTPCertificateTemplate(
				k8s.ExtractNamespacedName(&tt.args.es),
				esv1.ESNamer,
				tt.args.es.Spec.HTTP.TLS,
				tt.args.extraHTTPSANs,
				tt.args.svcs,
				&x509.CertificateRequest{},
				tt.args.certValidity,
			)
			if tt.want != nil {
				tt.want(t, got)
			}
		})
	}
}

func Test_getHTTPCertificate(t *testing.T) {
	esWithSAN := testES.DeepCopy()
	esWithSAN.Spec.HTTP = commonv1.HTTPConfig{
		TLS: commonv1.TLSOptions{
			SelfSignedCertificate: &commonv1.SelfSignedCertificate{
				SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
					{
						DNS: "search.example.com",
					},
				},
			},
		},
	}
	type args struct {
		es             esv1.Elasticsearch
		controllerSANs []commonv1.SubjectAlternativeName
		secret         corev1.Secret
		rotateBefore   time.Duration
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "missing cert in secret",
			args: args{
				secret:       corev1.Secret{},
				es:           testES,
				rotateBefore: DefaultRotateBefore,
			},
			want: nil,
		},
		{
			name: "invalid cert data",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: []byte("invalid"),
					},
				},
				es:           testES,
				rotateBefore: DefaultRotateBefore,
			},
			want: nil,
		},
		{
			name: "valid cert",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemTLS,
					},
				},
				es:           testES,
				rotateBefore: DefaultRotateBefore,
			},
			want: cert,
		},
		{
			name: "should be rotated soon",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemTLS,
					},
				},
				es:           testES,
				rotateBefore: DefaultCertValidity, // rotate before the same duration as total validity
			},
			want: nil,
		},
		{
			name: "with different SAN",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemTLS,
					},
				},
				es:           *esWithSAN,
				rotateBefore: DefaultRotateBefore,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getHTTPCertificate(
				context.Background(),
				k8s.ExtractNamespacedName(&tt.args.es),
				esv1.ESNamer,
				tt.args.es.Spec.HTTP.TLS,
				tt.args.controllerSANs,
				&tt.args.secret,
				[]corev1.Service{testSvc},
				testCA,
				tt.args.rotateBefore,
			); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}
