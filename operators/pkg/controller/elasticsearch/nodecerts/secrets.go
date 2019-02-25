// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
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

// ReconcileNodeCertificateSecret ensures that the node certificate secret has the available and correct Data keys
// provided.
func ReconcileNodeCertificateSecret(
	c k8s.Client,
	secret corev1.Secret,
	pod corev1.Pod,
	csrClient certificates.CSRClient,
	clusterName, namespace string,
	svcs []corev1.Service,
	ca *certificates.Ca,
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
	secret.Data[certificates.CAFileName] = certificates.EncodePEMCert(ca.Cert.Raw)
	for _, caPemBytes := range additionalTrustedCAsPemEncoded {
		secret.Data[certificates.CAFileName] = append(secret.Data[certificates.CAFileName], caPemBytes...)
	}
	secret.Data[CSRFileName] = csr
	secret.Data[CertFileName] = certificates.EncodePEMCert(certData, ca.Cert.Raw)
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
func shouldIssueNewCertificate(secret corev1.Secret, ca *certificates.Ca, pod corev1.Pod) bool {
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
