// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
	pemCert           []byte
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
		k8s.ExtractNamespacedName(&testES), esv1.ESNamer, testES.Spec.HTTP.TLS, []corev1.Service{testSvc}, testCSR, DefaultCertValidity,
	)

	certData, err := testCA.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		panic("Failed to create cert data:" + err.Error())
	}

	pemCert = EncodePEMCert(certData, testCA.Cert.Raw)
}

func TestReconcilePublicHTTPCerts(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")

	owner := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
	}

	certificate := &CertificatesSecret{
		Data: map[string][]byte{
			CAFileName:   ca,
			CertFileName: tls,
			KeyFileName:  key,
		},
	}

	namespacedSecretName := PublicCertsSecretRef(esv1.ESNamer, k8s.ExtractNamespacedName(owner))

	mkClient := func(t *testing.T, objs ...runtime.Object) k8s.Client {
		t.Helper()
		return k8s.WrappedFakeClient(objs...)
	}

	labels := map[string]string{"expected": "default-labels"}

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

		if err := controllerutil.SetControllerReference(owner, wantSecret, scheme.Scheme); err != nil {
			t.Fatal(err)
		}

		return wantSecret
	}

	tests := []struct {
		name       string
		client     func(*testing.T, ...runtime.Object) k8s.Client
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
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
				s := mkWantedSecret(t)
				s.Data[CertFileName] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "removes extraneous keys",
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
				s := mkWantedSecret(t)
				s.Data["extra"] = []byte{0, 1, 2, 3}
				return mkClient(t, s)
			},
			wantSecret: mkWantedSecret,
		},
		{
			name: "preserves labels and annotations",
			client: func(t *testing.T, _ ...runtime.Object) k8s.Client {
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
				Object:    owner,
				Namer:     esv1.ESNamer,
				Labels:    labels,
			}.ReconcilePublicHTTPCerts(certificate)
			if tt.wantErr {
				require.Error(t, err, "Failed to reconcile")
				return
			}

			var gotSecret corev1.Secret
			err = client.Get(namespacedSecretName, &gotSecret)
			require.NoError(t, err, "Failed to get secret")

			wantSecret := tt.wantSecret(t)
			comparison.AssertEqual(t, wantSecret, &gotSecret)
		})
	}
}

func TestReconcileInternalHTTPCerts(t *testing.T) {
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")

	type args struct {
		c        k8s.Client
		es       esv1.Elasticsearch
		ca       *CA
		services []corev1.Service
	}
	tests := []struct {
		name    string
		args    args
		want    func(t *testing.T, cs *CertificatesSecret)
		wantErr bool
	}{
		{
			name: "should generate new certificates if none exists",
			args: args{
				c:  k8s.WrappedFakeClient(),
				es: testES,
				ca: testCA,
			},
			want: func(t *testing.T, cs *CertificatesSecret) {
				assert.Contains(t, cs.Data, KeyFileName)
				assert.Contains(t, cs.Data, CertFileName)
			},
		},
		{
			name: "should use custom certificates if provided",
			args: args{
				c: k8s.WrappedFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "my-cert", Namespace: "test-namespace"},
					Data: map[string][]byte{
						CertFileName: tls,
						KeyFileName:  key,
					},
				}),
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
			},
			want: func(t *testing.T, cs *CertificatesSecret) {
				assert.Equal(t, cs.Data[KeyFileName], key)
				assert.Equal(t, cs.Data[CertFileName], tls)
				// Even if user didn't provide a CA cert we don't want it to be empty
				assert.True(t, len(cs.Data[CAFileName]) > 0)
				assert.Equal(t, cs.Data[CAFileName], EncodePEMCert(testCA.Cert.Raw))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := watches.NewDynamicWatches()
			got, err := Reconciler{
				K8sClient:      tt.args.c,
				DynamicWatches: w,
				Object:         &tt.args.es,
				TLSOptions:     tt.args.es.Spec.HTTP.TLS,
				Namer:          esv1.ESNamer,
				Labels:         map[string]string{},
				Services:       tt.args.services,
				CertRotation: RotationParams{
					Validity:     DefaultCertValidity,
					RotateBefore: DefaultRotateBefore,
				},
			}.ReconcileInternalHTTPCerts(tt.args.ca)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileInternalHTTPCerts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.want(t, got)
		})
	}
}

func Test_createValidatedHTTPCertificateTemplate(t *testing.T) {
	sanDNS1 := "my.dns.com"
	sanDNS2 := "my.second.dns.com"
	sanIP1 := "4.4.6.7"
	sanIPv6 := "2001:db8:0:85a3:0:0:ac1f:8001"

	type args struct {
		es           esv1.Elasticsearch
		svcs         []corev1.Service
		certValidity time.Duration
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
			},
			want: func(t *testing.T, cert *ValidatedCertificateTemplate) {
				expectedCommonName := "test-es-http.test.es.local"
				assert.Contains(t, cert.Subject.CommonName, expectedCommonName)
				assert.Contains(t, cert.DNSNames, expectedCommonName)
				assert.Contains(t, cert.DNSNames, "svc-name.svc-namespace.svc")
				assert.Contains(t, cert.DNSNames, sanDNS1)
				assert.Contains(t, cert.DNSNames, sanDNS2)
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

func Test_shouldIssueNewCertificate(t *testing.T) {
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
		es           esv1.Elasticsearch
		secret       corev1.Secret
		rotateBefore time.Duration
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "missing cert in secret",
			args: args{
				secret:       corev1.Secret{},
				es:           testES,
				rotateBefore: DefaultRotateBefore,
			},
			want: true,
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
			want: true,
		},
		{
			name: "valid cert",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemCert,
					},
				},
				es:           testES,
				rotateBefore: DefaultRotateBefore,
			},
			want: false,
		},
		{
			name: "should be rotated soon",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemCert,
					},
				},
				es:           testES,
				rotateBefore: DefaultCertValidity, // rotate before the same duration as total validity
			},
			want: true,
		},
		{
			name: "with different SAN",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						CertFileName: pemCert,
					},
				},
				es:           *esWithSAN,
				rotateBefore: DefaultRotateBefore,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldIssueNewHTTPCertificate(
				k8s.ExtractNamespacedName(&tt.args.es),
				esv1.ESNamer,
				tt.args.es.Spec.HTTP.TLS,
				&tt.args.secret,
				[]corev1.Service{testSvc},
				testCA,
				tt.args.rotateBefore,
			); got != tt.want {
				t.Errorf("shouldIssueNewCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}
