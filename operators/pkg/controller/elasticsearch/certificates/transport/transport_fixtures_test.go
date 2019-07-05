// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

// fixtures
var (
	testCA                       *certificates.CA
	testRSAPrivateKey            *rsa.PrivateKey
	testCSRBytes                 []byte
	testCSR                      *x509.CertificateRequest
	validatedCertificateTemplate *certificates.ValidatedCertificateTemplate
	certData                     []byte
	pemCert                      []byte
	testIP                       = "1.2.3.4"
	testES                       = v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "test-es-name", Namespace: "test-namespace"},
	}
	testPod = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod-name",
		},
		Status: corev1.PodStatus{
			PodIP: testIP,
		},
	}
	testSvc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "2.2.3.3",
		},
	}
	additionalCA = [][]byte{[]byte(testAdditionalCA)}
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
	testAdditionalCA = `-----BEGIN CERTIFICATE-----
MIIDKzCCAhOgAwIBAgIRAK7i/u/wsh+i2G0yUygsJckwDQYJKoZIhvcNAQELBQAw
LzEZMBcGA1UECxMQNG1jZnhjbnh0ZjZuNHA5bDESMBAGA1UEAxMJdHJ1c3Qtb25l
MB4XDTE5MDMyMDIwNDg1NloXDTIwMDMxOTIwNDk1NlowLzEZMBcGA1UECxMQNG1j
Znhjbnh0ZjZuNHA5bDESMBAGA1UEAxMJdHJ1c3Qtb25lMIIBIjANBgkqhkiG9w0B
AQEFAAOCAQ8AMIIBCgKCAQEAu/Pws5FcyJw843pNow/Y95rApWAuGanU99DEmeOG
ggtpc3qtDWWKwLZ6cU+av3u82tf0HYSpy0Z2hn3PS2dGGgHPTr/tTGYA5alu1dn5
CgqQDBVLbkKA1lDcm8w98fRavRw6a0TX5DURqXs+smhdMztQjDNCl3kJ40JbXVAY
x5vhD2pKPCK0VIr9uYK0E/9dvrU0SJGLUlB+CY/DU7c8t22oer2T6fjCZzh3Fhwi
/aOKEwEUoE49orte0N9b1HSKlVePzIUuTTc3UU2ntWi96Uf2FesuAubU11WH4kIL
wRlofty7ewBzVmGte1fKUMjHB3mgb+WYwkEFwjpQL4LhkQIDAQABo0IwQDAOBgNV
HQ8BAf8EBAMCAoQwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMA8GA1Ud
EwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAI+qczKQgkb5L5dXzn+KW92J
Sq1rrmaYUYLRTtPFH7t42REPYLs4UV0qR+6v/hJljQbAS+Vu3BioLWuxq85NsIjf
OK1KO7D8lwVI9tAetE0tKILqljTjwZpqfZLZ8fFqwzd9IM/WfoI7Z05k8BSL6XdM
FaRfSe/GJ+DR1dCwnWAVKGxAry4JSceVS9OXxYNRTcfQuT5s8h/6X5UaonTbhil7
91fQFaX8LSuZj23/3kgDTnjPmvj2sz5nODymI4YeTHLjdlMmTufWSJj901ITp7Bw
DMO3GhRADFpMz3vjHA2rHA4AQ6nC8N4lIYTw0AF1VAOC0SDntf6YEgrhRKRFAUY=
-----END CERTIFICATE-----`
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

	testCSRBytes, err = x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, testRSAPrivateKey)
	if err != nil {
		panic("Failed to create CSR:" + err.Error())
	}
	testCSR, err = x509.ParseCertificateRequest(testCSRBytes)

	validatedCertificateTemplate, err = createValidatedCertificateTemplate(
		testPod, testES, []corev1.Service{testSvc}, testCSR, certificates.DefaultCertValidity)
	if err != nil {
		panic("Failed to create validated cert template:" + err.Error())
	}

	certData, err = testCA.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		panic("Failed to create cert data:" + err.Error())
	}

	pemCert = certificates.EncodePEMCert(certData, testCA.Cert.Raw)
}
