// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts/certutil"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

	// LastCSRUpdateAnnotation is an annotation key to indicate the last time this secret's CSR was updated
	LastCSRUpdateAnnotation = "nodecerts.stack.k8s.elastic.co/last-csr-update"

	// CSRRequestDelay limits the number of CSR requests we do in consecutive reconciliations
	CSRRequestDelay = 1 * time.Minute
)

const (
	// CertFileName is used for the Certificates inside a secret
	CertFileName = "cert.pem"
	// CSRFileName is used for the CSR inside a secret
	CSRFileName = "csr.pem"
)

// NodeCertificateSecretObjectKeyForPod returns the object key for the secret containing the node certificates for
// a given pod.
func NodeCertificateSecretObjectKeyForPod(pod corev1.Pod) types.NamespacedName {
	// TODO: trim and suffix?
	return k8s.ExtractNamespacedName(&pod)
}

// EnsureNodeCertificateSecretExists ensures the existence of the corev1.Secret that at a later point in time will
// contain the node certificates.
func EnsureNodeCertificateSecretExists(
	c k8s.Client,
	scheme *runtime.Scheme,
	owner metav1.Object,
	pod corev1.Pod,
	nodeCertificateType string,
	labels map[string]string,
) (*corev1.Secret, error) {
	secretObjectKey := NodeCertificateSecretObjectKeyForPod(pod)

	var secret corev1.Secret
	if err := c.Get(secretObjectKey, &secret); err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	} else if apierrors.IsNotFound(err) {
		secret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
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

		// apply any provided labels
		for key, value := range labels {
			secret.Labels[key] = value
		}

		if err := controllerutil.SetControllerReference(owner, &secret, scheme); err != nil {
			return nil, err
		}

		if err := c.Create(&secret); err != nil {
			return nil, err
		}
	}

	// TODO: in the future we should consider reconciling the existing secret as well instead of leaving it untouched.
	return &secret, nil
}

// maybeRequestCSR requests the pod for a new CSR if required, or returns nil.
func maybeRequestCSR(pod corev1.Pod, csrClient nodecerts.CSRClient, lastCSRUpdate string) ([]byte, error) {
	// If the CSR secret was updated very recently, chances are we already issued a new certificate
	// which has not yet been propagated to the pod (it can take more than 1 minute).
	// In such case, there is no need to request the same CSR again and again at each reconciliation.
	lastUpdate, err := time.Parse(time.RFC3339, lastCSRUpdate)
	if err != nil {
		log.V(1).Info("lastCSRUpdate time cannot be parsed, probably because not set yet. Ignoring.", "pod", pod.Name)
	} else {
		delay := time.Now().Sub(lastUpdate)
		if delay > 0 && delay < CSRRequestDelay {
			log.V(1).Info("CSR was already updated recently, let's wait before requesting a new one", "pod", pod.Name)
			return nil, nil
		}
	}
	// Check status of the pod's cert-initializer init container: if running, it's waiting for
	// a valid certificate to be issued, hence we should request a new CSR.
	for _, c := range pod.Status.InitContainerStatuses {
		if c.Name == initcontainer.CertInitializerContainerName && c.State.Running != nil {
			newCSR, err := csrClient.RetrieveCSR(pod)
			if err != nil && err != io.EOF { // EOF is ok, just the cert-initializer shutting down
				return nil, err
			}
			return newCSR, nil
		}
	}
	return nil, nil
}

// ReconcileNodeCertificateSecret ensures that the node certificate secret has the available and correct Data keys
// provided.
func ReconcileNodeCertificateSecret(
	c k8s.Client,
	secret corev1.Secret,
	pod corev1.Pod,
	csrClient nodecerts.CSRClient,
	clusterName, namespace string,
	svcs []corev1.Service,
	ca *nodecerts.Ca,
	additionalTrustedCAsPemEncoded [][]byte,
) (reconcile.Result, error) {
	// a placeholder secret may have nil entries, create them if needed
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}

	csr := secret.Data[CSRFileName]                              // may be nil
	lastCSRUpdate := secret.Annotations[LastCSRUpdateAnnotation] // may be empty

	// check if the existing cert is correct
	issueNewCertificate := shouldIssueNewCertificate(secret, ca, pod)

	// if needed, replace the CSR by a fresh one
	newCSR, err := maybeRequestCSR(pod, csrClient, lastCSRUpdate)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(newCSR) > 0 && !bytes.Equal(csr, newCSR) {
		// pod issued a new CSR, probably generated from a new private key
		// we should issue a new cert for this CSR
		csr = newCSR
		issueNewCertificate = true
		lastCSRUpdate = time.Now().Format(time.RFC3339)
	}

	if len(csr) == 0 {
		// no csr yet, let's requeue until cert-initializer is available
		return reconcile.Result{}, nil
	}

	if !issueNewCertificate {
		// nothing to do, we're all set
		return reconcile.Result{}, nil
	}

	log.Info(
		"Issuing new certificate",
		"secret", secret.Name,
		"clusterName", clusterName,
		"namespace", namespace,
	)

	// create a cert from the csr
	parsedCSR, err := x509.ParseCertificateRequest(csr)
	if err != nil {
		return reconcile.Result{}, err
	}
	validatedCertificateTemplate, err := CreateValidatedCertificateTemplate(pod, clusterName, namespace, svcs, parsedCSR)
	if err != nil {
		return reconcile.Result{}, err
	}
	// sign the certificate
	certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		return reconcile.Result{}, err
	}

	// store CA cert, CSR and signed certificate in a secret mounted into the pod
	secret.Data[nodecerts.CAFileName] = certutil.EncodePEMCert(ca.Cert.Raw)
	for _, caPemBytes := range additionalTrustedCAsPemEncoded {
		secret.Data[nodecerts.CAFileName] = append(secret.Data[nodecerts.CAFileName], caPemBytes...)
	}
	secret.Data[CSRFileName] = csr
	secret.Data[CertFileName] = certutil.EncodePEMCert(certData, ca.Cert.Raw)
	// store last CSR update in the pod annotations
	secret.Annotations[LastCSRUpdateAnnotation] = lastCSRUpdate

	log.Info("Updating node certificate secret", "secret", secret.Name)
	if err := c.Update(&secret); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// shouldIssueNewCertificate returns true if we should issue a new certificate.
// Reasons for reissuing a certificate:
// - no certificate yet
// - certificate has the wrong format
// - certificate is invalid or expired
// - certificate SAN and IP does not match pod SAN and IP
func shouldIssueNewCertificate(secret corev1.Secret, ca *nodecerts.Ca, pod corev1.Pod) bool {
	certData, ok := secret.Data[CertFileName]
	if !ok {
		// certificate is missing
		return true
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		log.Info("Invalid certificate data found, issuing new certificate", "secret", secret.Name)
		return true
	}
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
	}

	// TODO: verify expected SANs in certificate, otherwise we wont actually reconcile such changes

	return false
}

// CreateValidatedCertificateTemplate validates a CSR and creates a certificate template.
func CreateValidatedCertificateTemplate(
	pod corev1.Pod,
	clusterName, namespace string,
	svcs []corev1.Service,
	csr *x509.CertificateRequest,
) (*nodecerts.ValidatedCertificateTemplate, error) {
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

	certificateTemplate := nodecerts.ValidatedCertificateTemplate(x509.Certificate{
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
