package nodecerts

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts/certutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("nodecerts")
)

const (
	// LabelAssociatedPod is a label key that indicates the resource is supposed to have a named associated pod
	LabelAssociatedPod = "nodecerts.stack.k8s.elastic.co/associated-pod"

	// LabelSecretUsage is a label key that specifies what the secret is used for
	LabelSecretUsage = "nodecerts.stack.k8s.elastic.co/secret-usage"
	// LabelSecretUsageNodeCertificates is the LabelSecretUsage value used for node certificates
	LabelSecretUsageNodeCertificates = "node-certificates"

	// LabelNodeCertificateType is a label key indicating what the node-certificates secret is used for
	LabelNodeCertificateType = "nodecerts.stack.k8s.elastic.co/node-certificate-type"
	// LabelNodeCertificateTypeElasticsearchAll is the LabelNodeCertificateType value used for Elasticsearch
	LabelNodeCertificateTypeElasticsearchAll = "elasticsearch.all"
)

const (
	// SecretCAKey is used for the CA Certificates inside a secret
	SecretCAKey = "ca.pem"
	// SecretCertKey is used for the Certificates inside a secret
	SecretCertKey = "cert.pem"
	// SecretPrivateKeyKey is used for the private keys inside a secret
	SecretPrivateKeyKey = "node.key"
)

const (
	// BlockTypeRSAPrivateKey is the PEM preamble type for an RSA private key
	BlockTypeRSAPrivateKey = "RSA PRIVATE KEY"
	// BlockTypeCertificate is the PEM preamble type for an X509 certificate
	BlockTypeCertificate = "CERTIFICATE"
)

// NodeCertificateSecretObjectKeyForPod returns the object key for the secret containing the node certificates for
// a given pod.
func NodeCertificateSecretObjectKeyForPod(pod corev1.Pod) types.NamespacedName {
	// TODO: trim and suffix?
	return types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}
}

// EnsureNodeCertificateSecretExists ensures that the corev1.Secret that at a later point in time will contain the node
// certificates exists.
func EnsureNodeCertificateSecretExists(
	c client.Client,
	scheme *runtime.Scheme,
	owner v1.Object,
	pod corev1.Pod,
	nodeCertificateType string,
) (*corev1.Secret, error) {
	secretObjectKey := NodeCertificateSecretObjectKeyForPod(pod)

	var secret corev1.Secret
	if err := c.Get(context.TODO(), secretObjectKey, &secret); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if apierrors.IsNotFound(err) {
		secret = corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      secretObjectKey.Name,
				Namespace: secretObjectKey.Namespace,

				Labels: map[string]string{
					// store the pod that this Secret will be mounted to so we can traverse from secret -> pod
					LabelAssociatedPod:       pod.Name,
					LabelSecretUsage:         LabelSecretUsageNodeCertificates,
					LabelNodeCertificateType: nodeCertificateType,
				},
			},
		}

		if err := controllerutil.SetControllerReference(owner, &secret, scheme); err != nil {
			return nil, err
		}

		if err := c.Create(context.TODO(), &secret); err != nil {
			return nil, err
		}
	}

	return &secret, nil
}

// ReconcileNodeCertificateSecret ensures that the node certificate secret has the available and correct Data keys
// provided.
//
// TODO: method should not generate the private key
// TODO: method should take a CSR argument instead of creating it
func ReconcileNodeCertificateSecret(
	secret corev1.Secret,
	pod corev1.Pod,
	clusterName, namespace string,
	svcs []corev1.Service,
	ca *Ca,
	c client.Client,
) (reconcile.Result, error) {
	// a placeholder secret may have a nil secret.Data, so create it if it does not exist
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	// XXX: be a little crazy, live a little. push private keys over the network.
	if _, ok := secret.Data[SecretPrivateKeyKey]; !ok {
		key, err := rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "unable to generate the private key")
		}

		pemKeyBytes := pem.EncodeToMemory(&pem.Block{
			Type:  BlockTypeRSAPrivateKey,
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})

		secret.Data[SecretPrivateKeyKey] = pemKeyBytes
	}

	issueNewCertificate := shouldIssueNewCertificate(secret, ca, pod)

	if issueNewCertificate {
		log.Info("Issuing new certificate", "secret", secret.Name)

		block, _ := pem.Decode(secret.Data[SecretPrivateKeyKey])
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return reconcile.Result{}, err
		}

		cr := x509.CertificateRequest{}
		csrBytes, err := x509.CreateCertificateRequest(cryptorand.Reader, &cr, key)
		if err != nil {
			return reconcile.Result{}, err
		}

		csr, err := x509.ParseCertificateRequest(csrBytes)
		if err != nil {
			return reconcile.Result{}, err
		}

		validatedCertificateTemplate, err := createValidatedCertificateTemplate(pod, clusterName, namespace, svcs, csr)
		if err != nil {
			return reconcile.Result{}, err
		}

		certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
		if err != nil {
			return reconcile.Result{}, err
		}

		secret.Data[SecretCAKey] = pem.EncodeToMemory(&pem.Block{Type: BlockTypeCertificate, Bytes: ca.Cert.Raw})
		secret.Data[SecretCertKey] = append(
			pem.EncodeToMemory(&pem.Block{Type: BlockTypeCertificate, Bytes: certData}),
			pem.EncodeToMemory(&pem.Block{Type: BlockTypeCertificate, Bytes: ca.Cert.Raw})...,
		)

		if err := c.Update(context.TODO(), &secret); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// shouldIssueNewCertificate returns true if we should issue a new certificate.
func shouldIssueNewCertificate(secret corev1.Secret, ca *Ca, pod corev1.Pod) bool {
	// could simplify this block to just checking if the ca.pem file contents would be the same, but it may be more
	// likely that we'd like to embellish this logic further?

	if certData, ok := secret.Data[SecretCertKey]; !ok {
		// certificate missing
		return true
	} else {
		block, _ := pem.Decode(certData)
		if block == nil {
			log.Info("Invalid certificate data found, issuing new certificate", "secret", secret.Name)
			return true
		} else {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				log.Info("Invalid certificate found as first block, issuing new certificate", "secret", secret.Name)
				return true
			}

			pool := x509.NewCertPool()
			pool.AddCert(ca.Cert)

			verifyOpts := x509.VerifyOptions{
				DNSName:       pod.Name,
				Roots:         pool,
				Intermediates: pool,
			}
			if _, err := cert.Verify(verifyOpts); err != nil {
				log.Info(
					fmt.Sprintf("Certificate was not valid, should issue new: %s", err),
					"subject", cert.Subject,
					"issuer", cert.Issuer,
					"current_ca_subject", ca.Cert.Subject,
				)
				return true
			} else {
				// TODO: verify expected SANs in certificate, otherwise we wont actually reconcile such changes
			}
		}
	}
	return false
}

// createValidatedCertificateTemplate validates a CSR and creates a certificate template
func createValidatedCertificateTemplate(
	pod corev1.Pod,
	clusterName, namespace string,
	svcs []corev1.Service,
	csr *x509.CertificateRequest,
) (*ValidatedCertificateTemplate, error) {
	podIp := net.ParseIP(pod.Status.PodIP)
	if podIp == nil {
		return nil, fmt.Errorf("pod currently has no valid IP, found: [%s]", pod.Status.PodIP)
	}

	commonName := buildCertificateCommonName(pod, clusterName, namespace)
	commonNameUTF8OtherName := &certutil.UTF8StringValuedOtherName{
		OID:   certutil.CommonNameObjectIdentifier,
		Value: commonName,
	}
	commonNameOtherName, err := commonNameUTF8OtherName.ToOtherName()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create othername")
	}

	// because we're using the ES-customized subject alternative-names extension, we have to handle all the general
	// names here instead of using x509.Certificate.DNSNames, .IPAddresses etc.
	generalNames := []certutil.GeneralName{
		{OtherName: *commonNameOtherName},
		{DNSName: commonName},
		{DNSName: pod.Name},
		{IPAddress: maybeIPTo4(podIp)},
		{IPAddress: net.ParseIP("127.0.0.1").To4()},
	}

	if svcs != nil {
		for _, svc := range svcs {
			if ip := net.ParseIP(svc.Spec.ClusterIP); ip != nil {
				generalNames = append(generalNames,
					certutil.GeneralName{IPAddress: maybeIPTo4(ip)},
				)
			}

			generalNames = append(generalNames,
				certutil.GeneralName{DNSName: svc.Name},
				certutil.GeneralName{DNSName: getServiceFullyQualifiedHostname(svc)},
			)
		}
	}

	generalNamesBytes, err := certutil.MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		return nil, err
	}

	// TODO: csr signature is not checked, common name not verified
	// TODO: add services dns entries / ip addresses to cert?

	certificateTemplate := ValidatedCertificateTemplate(x509.Certificate{
		Subject: pkix.Name{
			CommonName:         commonName,
			OrganizationalUnit: []string{clusterName},
		},

		ExtraExtensions: []pkix.Extension{
			{Id: certutil.SubjectAlternativeNamesObjectIdentifier, Value: generalNamesBytes},
		},
		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		Signature:          csr.Signature,
		SignatureAlgorithm: csr.SignatureAlgorithm,

		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	})

	return &certificateTemplate, nil
}

// buildCertificateCommonName returns the CN (and ES othername) entry for a given pod within a stack
// this needs to be kept in sync with the usage of trust_restrictions (see elasticsearch.TrustConfig)
func buildCertificateCommonName(pod corev1.Pod, clusterName, namespace string) string {
	return fmt.Sprintf("%s.node.%s.%s.es.cluster.local", pod.Name, clusterName, namespace)
}

// getServiceFullyQualifiedHostname returns the fully qualified DNS name for a service
func getServiceFullyQualifiedHostname(svc corev1.Service) string {
	// TODO: cluster.local suffix should be configurable
	return fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace)
}

// maybeIPTo4 attempts to convert the provided net.IP to a 4-byte representation if possible, otherwise does nothing.
func maybeIPTo4(ipAddress net.IP) net.IP {
	if ip := ipAddress.To4(); ip != nil {
		return ip
	}
	return ipAddress
}
