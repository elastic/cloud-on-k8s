// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"testing"
	"time"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	testCA            *certificates.CA
	testRSAPrivateKey *rsa.PrivateKey
	pemCert           []byte
	testES            = v1alpha1.Elasticsearch{ObjectMeta: v1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"}}
	testSvc           = corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "2.2.3.3",
		},
	}
)

func init() {
	if err := v1alpha1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}

	var err error
	block, _ := pem.Decode([]byte(testPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse private key: " + err.Error())
	}

	if testCA, err = certificates.NewSelfSignedCA(certificates.CABuilderOptions{
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
		k8s.ExtractNamespacedName(&testES), name.ESNamer, testES.Spec.HTTP.TLS, []corev1.Service{testSvc}, testCSR, certificates.DefaultCertValidity,
	)

	certData, err := testCA.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		panic("Failed to create cert data:" + err.Error())
	}

	pemCert = certificates.EncodePEMCert(certData, testCA.Cert.Raw)
}

func TestReconcileHTTPCertificates(t *testing.T) {
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")

	type args struct {
		c        k8s.Client
		es       v1alpha1.Elasticsearch
		ca       *certificates.CA
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
				c:  k8s.WrapClient(fake.NewFakeClient()),
				es: testES,
				ca: testCA,
			},
			want: func(t *testing.T, cs *CertificatesSecret) {
				assert.Contains(t, cs.Data, certificates.KeyFileName)
				assert.Contains(t, cs.Data, certificates.CertFileName)
			},
		},
		{
			name: "should use custom certificates if provided",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
					ObjectMeta: v1.ObjectMeta{Name: "my-cert", Namespace: "test-namespace"},
					Data: map[string][]byte{
						certificates.CertFileName: tls,
						certificates.KeyFileName:  key,
					},
				})),
				es: v1alpha1.Elasticsearch{
					ObjectMeta: v1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
					Spec: v1alpha1.ElasticsearchSpec{
						HTTP: commonv1alpha1.HTTPConfig{
							TLS: commonv1alpha1.TLSOptions{
								Certificate: commonv1alpha1.SecretRef{
									SecretName: "my-cert",
								},
							},
						},
					},
				},
				ca: testCA,
			},
			want: func(t *testing.T, cs *CertificatesSecret) {
				assert.Equal(t, cs.Data[certificates.KeyFileName], key)
				assert.Equal(t, cs.Data[certificates.CertFileName], tls)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := watches.NewDynamicWatches()
			require.NoError(t, w.InjectScheme(scheme.Scheme))
			testDriver := driver.TestDriver{
				Client:        tt.args.c,
				RuntimeScheme: scheme.Scheme,
				Watches:       w,
			}

			got, err := ReconcileHTTPCertificates(
				testDriver, &tt.args.es, name.ESNamer, tt.args.ca, tt.args.es.Spec.HTTP.TLS, map[string]string{}, tt.args.services,
				certificates.RotationParams{
					Validity:     certificates.DefaultCertValidity,
					RotateBefore: certificates.DefaultRotateBefore,
				},
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileHTTPCertificates() error = %v, wantErr %v", err, tt.wantErr)
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
		es           v1alpha1.Elasticsearch
		svcs         []corev1.Service
		certValidity time.Duration
	}
	tests := []struct {
		name string
		args args
		want func(t *testing.T, cert *certificates.ValidatedCertificateTemplate)
	}{
		{
			name: "with svcs and user-provided SANs",
			args: args{
				es: v1alpha1.Elasticsearch{
					ObjectMeta: v1.ObjectMeta{Namespace: "test", Name: "test"},
					Spec: v1alpha1.ElasticsearchSpec{
						HTTP: commonv1alpha1.HTTPConfig{
							TLS: commonv1alpha1.TLSOptions{
								SelfSignedCertificate: &commonv1alpha1.SelfSignedCertificate{
									SubjectAlternativeNames: []commonv1alpha1.SubjectAlternativeName{
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
						ObjectMeta: v1.ObjectMeta{
							Namespace: "svc-namespace",
							Name:      "svc-name",
						},
						Spec: corev1.ServiceSpec{
							ClusterIP: "10.11.12.13",
						},
					},
				},
			},
			want: func(t *testing.T, cert *certificates.ValidatedCertificateTemplate) {
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
				name.ESNamer,
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
	esWithSAN.Spec.HTTP = commonv1alpha1.HTTPConfig{
		TLS: commonv1alpha1.TLSOptions{
			SelfSignedCertificate: &commonv1alpha1.SelfSignedCertificate{
				SubjectAlternativeNames: []commonv1alpha1.SubjectAlternativeName{
					{
						DNS: "search.example.com",
					},
				},
			},
		},
	}
	type args struct {
		es           v1alpha1.Elasticsearch
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
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "invalid cert data",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: []byte("invalid"),
					},
				},
				es:           testES,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
		{
			name: "valid cert",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: pemCert,
					},
				},
				es:           testES,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: false,
		},
		{
			name: "should be rotated soon",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: pemCert,
					},
				},
				es:           testES,
				rotateBefore: certificates.DefaultCertValidity, // rotate before the same duration as total validity
			},
			want: true,
		},
		{
			name: "with different SAN",
			args: args{
				secret: corev1.Secret{
					Data: map[string][]byte{
						certificates.CertFileName: pemCert,
					},
				},
				es:           *esWithSAN,
				rotateBefore: certificates.DefaultRotateBefore,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldIssueNewHTTPCertificate(
				k8s.ExtractNamespacedName(&tt.args.es),
				name.ESNamer,
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
