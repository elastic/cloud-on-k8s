package nodecerts

import (
	"crypto/x509"
	"net"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts/certutil"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
)

func Test_createValidatedCertificateTemplate(t *testing.T) {
	es := v1alpha1.ElasticsearchCluster{
		ObjectMeta: k8s.ObjectMeta("test-namespace", "test-es-name"),
	}
	testIp := "1.2.3.4"
	pod := v1.Pod{
		ObjectMeta: k8s.ObjectMeta("", "test-pod-name"),
		Status: v1.PodStatus{
			PodIP: testIp,
		},
	}
	csr := x509.CertificateRequest{
		PublicKeyAlgorithm: x509.RSA,
		PublicKey:          &testRSAPrivateKey.PublicKey,
	}

	svc := v1.Service{
		ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "test-service"),
		Spec: v1.ServiceSpec{
			ClusterIP: "2.2.3.3",
		},
	}

	validatedCert, err := createValidatedCertificateTemplate(pod, es.Name, es.Namespace, []v1.Service{svc}, &csr)
	require.NoError(t, err)

	// roundtrip the certificate
	certRT, err := roundTripSerialize(validatedCert)
	require.NoError(t, err)
	require.NotNil(t, certRT, "roundtripped certificate should not be nil")

	// regular dns names and ip addresses should be present in the cert
	assert.Contains(t, certRT.DNSNames, pod.Name)
	assert.Contains(t, certRT.IPAddresses, net.ParseIP(testIp).To4())
	assert.Contains(t, certRT.IPAddresses, net.ParseIP("127.0.0.1").To4())

	// service ip and hosts should be present in the cert
	assert.Contains(t, certRT.IPAddresses, net.ParseIP(svc.Spec.ClusterIP).To4())
	assert.Contains(t, certRT.DNSNames, svc.Name)
	assert.Contains(t, certRT.DNSNames, getServiceFullyQualifiedHostname(svc))

	// es specific othernames is a bit more difficult to get to, but should be present:
	otherNames, err := certutil.ParseSANGeneralNamesOtherNamesOnly(certRT)
	require.NoError(t, err)

	// we expect this name to be used for both the common name as well as the es othername
	cn := "test-pod-name.node.test-es-name.test-namespace.es.cluster.local"

	otherName, err := (&certutil.UTF8StringValuedOtherName{
		OID:   certutil.CommonNameObjectIdentifier,
		Value: cn,
	}).ToOtherName()
	require.NoError(t, err)

	assert.Equal(t, certRT.Subject.CommonName, cn)
	assert.Contains(t, otherNames, certutil.GeneralName{OtherName: *otherName})
}

// roundTripSerialize does a serialization round-trip of the certificate in order to make sure any extra extensions
// are parsed and considered part of the certificate
func roundTripSerialize(cert *ValidatedCertificateTemplate) (*x509.Certificate, error) {
	certData, err := testCa.CreateCertificate(*cert)
	if err != nil {
		return nil, err
	}
	certRT, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return certRT, nil
}
