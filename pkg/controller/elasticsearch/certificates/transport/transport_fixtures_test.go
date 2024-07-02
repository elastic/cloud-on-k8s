// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/pem"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
)

const (
	testNamespace = "test-namespace"
	testEsName    = "test-es-name"
)

// fixtures
var (

	//go:embed testdata/ca.crt
	extraCA []byte

	// RSA related fixtures
	testRSACA            *certificates.CA
	testRSACABytes       []byte
	testRSAPrivateKey    *rsa.PrivateKey
	testRSAPEMPrivateKey []byte
	testRSACSR           *x509.CertificateRequest
	rsaCert              []byte

	// ECDSA related fixtures
	testECDSAPEMPrivateKey []byte

	testIP = "1.2.3.4"
	testES = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: testEsName, Namespace: testNamespace},
	}
	testPod = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod-name",
			Labels: map[string]string{
				label.StatefulSetNameLabelName: "test-sset",
			},
		},
		Status: corev1.PodStatus{
			PodIP: testIP,
		},
	}
)

const (
	testRSAPemPrivateKey = `
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

	testECDSAPemPrivateKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIFinrnCoEdQgo/WtQGddVtwztnURKqPhN7MIzCqfMaQEoAoGCCqGSM49
AwEHoUQDQgAEfAHH+RDvB5pKW9O4QsxEd9WHvQFcYQRXuFWBg8RY/+wTx2FjcofM
Ga4a1ldFfW870nGUulQplXN+uukCfsTiEw==
-----END EC PRIVATE KEY-----`
)

func init() {
	var err error
	block, _ := pem.Decode([]byte(testRSAPemPrivateKey))
	if testRSAPrivateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
		panic("Failed to parse RSA private key: " + err.Error())
	}

	if testRSAPEMPrivateKey, err = certificates.EncodePEMPrivateKey(testRSAPrivateKey); err != nil {
		panic("Failed to encode RSA private key: " + err.Error())
	}

	if testRSACA, err = certificates.NewSelfSignedCA(certificates.CABuilderOptions{
		Subject:    pkix.Name{CommonName: "test-common-name"},
		PrivateKey: testRSAPrivateKey,
	}); err != nil {
		panic("Failed to create new self signed CA: " + err.Error())
	}

	testRSACABytes = certificates.EncodePEMCert(testRSACA.Cert.Raw)

	testRSACSRBytes, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, testRSAPrivateKey)
	if err != nil {
		panic("Failed to create CSR:" + err.Error())
	}
	testRSACSR, err = x509.ParseCertificateRequest(testRSACSRBytes)
	if err != nil {
		panic("Failed to parse CSR:" + err.Error())
	}

	validatedRSACertificateTemplate, err := createValidatedCertificateTemplate(testPod, testES, testRSACSR, certificates.DefaultCertValidity)
	if err != nil {
		panic("Failed to create validated cert template:" + err.Error())
	}

	rsaCertData, err := testRSACA.CreateCertificate(*validatedRSACertificateTemplate)
	if err != nil {
		panic("Failed to create cert data:" + err.Error())
	}

	rsaCert = certificates.EncodePEMCert(rsaCertData, testRSACA.Cert.Raw)

	block, _ = pem.Decode([]byte(testECDSAPemPrivateKey))
	testECDSAPrivateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		panic("Failed to parse ECDSA private key: " + err.Error())
	}

	if testECDSAPEMPrivateKey, err = certificates.EncodePEMPrivateKey(testECDSAPrivateKey); err != nil {
		panic("Failed to encode private key: " + err.Error())
	}
}

// -- Elasticsearch builder

type esBuilder struct {
	nodeSets               []esv1.NodeSet
	transportCertsDisabled bool
}

func newEsBuilder() *esBuilder {
	return &esBuilder{}
}

func (eb *esBuilder) addNodeSet(name string, count int) *esBuilder {
	eb.nodeSets = append(eb.nodeSets, esv1.NodeSet{
		Name:  name,
		Count: int32(count),
	})
	return eb
}

func (eb *esBuilder) disableTransportCerts() *esBuilder {
	eb.transportCertsDisabled = true
	return eb
}

func (eb *esBuilder) build() *esv1.Elasticsearch {
	es := testES.DeepCopy()
	es.Spec.NodeSets = eb.nodeSets
	es.Spec.Transport.TLS.SelfSignedCertificates = &esv1.SelfSignedTransportCertificates{Disabled: eb.transportCertsDisabled}
	return es
}

// -- Transport Certs Secret builder

type transportCertsSecretBuilder struct {
	statefulset string
	data        map[string][]byte
}

// newtransportCertsSecretBuilder helps to create an existing Secret which contains some transport certs.
func newtransportCertsSecretBuilder(esName string, nodeSetName string) *transportCertsSecretBuilder {
	tcb := &transportCertsSecretBuilder{}
	tcb.statefulset = esv1.StatefulSet(esName, nodeSetName)
	tcb.data = make(map[string][]byte)
	caBytes := certificates.EncodePEMCert(testRSACA.Cert.Raw)
	tcb.data[certificates.CAFileName] = caBytes
	return tcb
}

// forPodIndices adds a transport cert for the pod in the StatefulSet with the given index
func (tcb *transportCertsSecretBuilder) forPodIndices(indices ...int) *transportCertsSecretBuilder {
	for _, index := range indices {
		podName := sset.PodName(tcb.statefulset, int32(index))
		tcb.data[PodKeyFileName(podName)] = testRSAPEMPrivateKey
		tcb.data[PodCertFileName(podName)] = rsaCert
	}
	return tcb
}

func (tcb *transportCertsSecretBuilder) build() *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      esv1.StatefulSetTransportCertificatesSecret(tcb.statefulset),
		},
	}
	secret.Data = tcb.data
	return secret
}

// -- Pod builder

type podBuilder struct {
	es          string
	ip          string
	nodeSet     string
	index       int
	annotations map[string]string
}

func newPodBuilder() *podBuilder {
	return &podBuilder{}
}

func (pb *podBuilder) forEs(es string) *podBuilder {
	pb.es = es
	return pb
}

func (pb *podBuilder) inNodeSet(nodeSet string) *podBuilder {
	pb.nodeSet = nodeSet
	return pb
}

func (pb *podBuilder) withIndex(index int) *podBuilder {
	pb.index = index
	return pb
}

func (pb *podBuilder) withIP(ip string) *podBuilder {
	pb.ip = ip
	return pb
}

func (pb *podBuilder) withAnnotations(a map[string]string) *podBuilder {
	pb.annotations = a
	return pb
}

func (pb *podBuilder) build() *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      fmt.Sprintf("%s-%d", esv1.StatefulSet(pb.es, pb.nodeSet), pb.index),
			Labels: map[string]string{
				label.StatefulSetNameLabelName: esv1.StatefulSet(pb.es, pb.nodeSet),
				label.ClusterNameLabelName:     pb.es,
			},
			Annotations: pb.annotations,
			UID:         uuid.NewUUID(),
		},
	}
	if len(pb.ip) > 0 {
		pod.Status.PodIP = pb.ip
	}
	return pod
}

func getSecret(list corev1.SecretList, name string) *corev1.Secret {
	for _, s := range list.Items {
		if s.Name == name {
			return &s
		}
	}
	return nil
}

func newStatefulSet(esName, ssetName string) *v1.StatefulSet {
	return &v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      ssetName,
			Labels: map[string]string{
				"elasticsearch.k8s.elastic.co/statefulset-name": ssetName,
				"common.k8s.elastic.co/type":                    "elasticsearch",
				"elasticsearch.k8s.elastic.co/cluster-name":     esName,
			},
		},
	}
}
