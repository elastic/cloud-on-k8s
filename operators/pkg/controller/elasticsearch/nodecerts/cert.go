// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.KBLog.WithName("nodecerts")

// Node certificates
//
// For each elasticsearch pod, we sign one certificate with the cluster CA (see `ca.go`).
// The certificate is passed to the pod through a secret volume mount.
// The corresponding private key stays in the ES pod: we request a CSR from the pod,
// and never access the private key directly.

const (
	lastCertUpdateAnnotation = "elasticsearch.k8s.elastic.co/last-cert-update"
)

// ReconcileNodeCertificateSecrets reconciles certificate secrets for nodes
// of the given es cluster.
func ReconcileNodeCertificateSecrets(
	c k8s.Client,
	ca *certificates.CA,
	csrClient certificates.CSRClient,
	es v1alpha1.Elasticsearch,
	services []corev1.Service,
	trustRelationships []v1alpha1.TrustRelationship,
	nodeCertValidity time.Duration,
	nodeCertRotateBefore time.Duration,
) (reconcile.Result, error) {
	log.Info("Reconciling node certificate secrets")

	additionalCAs := make([][]byte, 0, len(trustRelationships))
	for _, trustRelationship := range trustRelationships {
		if trustRelationship.Spec.CaCert == "" {
			continue
		}

		additionalCAs = append(additionalCAs, []byte(trustRelationship.Spec.CaCert))
	}

	// get all existing secrets for this cluster
	nodeCertificateSecrets, err := findNodeCertificateSecrets(c, es)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, secret := range nodeCertificateSecrets {
		// retrieve pod associated to this secret
		podName, ok := secret.Labels[LabelAssociatedPod]
		if !ok {
			return reconcile.Result{}, fmt.Errorf("Cannot find pod name in labels of secret %s", secret.Name)
		}

		var pod corev1.Pod
		if err := c.Get(types.NamespacedName{Namespace: secret.Namespace, Name: podName}, &pod); err != nil {
			if !apierrors.IsNotFound(err) {
				return reconcile.Result{}, err
			}

			// pod does not exist anymore, garbage-collect the secret
			// give some leniency in pods showing up only after a while
			if secret.CreationTimestamp.Add(5 * time.Minute).Before(time.Now()) {
				// if the secret has existed for too long without an associated pod, it's time to GC it
				log.Info("Unable to find pod associated with secret, GCing", "secret", secret.Name)
				if err := c.Delete(&secret); err != nil {
					return reconcile.Result{}, err
				}
			} else {
				log.Info("Unable to find pod associated with secret, but secret is too young for GC", "secret", secret.Name)
			}
			continue
		}

		if pod.Status.PodIP == "" {
			log.Info("Skipping secret because associated pod has no pod ip", "secret", secret.Name)
			continue
		}

		certificateType, ok := secret.Labels[LabelNodeCertificateType]
		if !ok {
			log.Error(errors.New("missing certificate type"), "No certificate type found", "secret", secret.Name)
			continue
		}

		switch certificateType {
		case LabelNodeCertificateTypeElasticsearchAll:
			if res, err := doReconcile(
				c, secret, pod, csrClient, es.Name, es.Namespace, services, ca, additionalCAs, nodeCertValidity, nodeCertRotateBefore,
			); err != nil {
				return res, err
			}
		default:
			log.Error(
				errors.New("unsupported certificate type"),
				fmt.Sprintf("Unsupported cerificate type: %s found in %s, ignoring", certificateType, secret.Name),
			)
		}
	}

	return reconcile.Result{}, nil
}

// doReconcile ensures that the node certificate secret has the correct content.
func doReconcile(
	c k8s.Client,
	secret corev1.Secret,
	pod corev1.Pod,
	csrClient certificates.CSRClient,
	clusterName, namespace string,
	svcs []corev1.Service,
	ca *certificates.CA,
	additionalTrustedCAsPemEncoded [][]byte,
	nodeCertValidity time.Duration,
	nodeCertReconcileBefore time.Duration,
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
	issueNewCertificate := shouldIssueNewCertificate(secret, ca, pod, nodeCertReconcileBefore)

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
	validatedCertificateTemplate, err := CreateValidatedCertificateTemplate(pod, clusterName, namespace, svcs, parsedCSR, nodeCertValidity)
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

	// To speedup secret propagation into the pod, also update the pod itself
	// with a "dummy" annotation. Otherwise, it may take 1+ minute.
	// This could be fixed in kubelet at some point,
	// see https://github.com/kubernetes/kubernetes/issues/30189
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[lastCertUpdateAnnotation] = time.Now().Format(time.RFC3339)
	if err := c.Update(&pod); err != nil {
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
func shouldIssueNewCertificate(secret corev1.Secret, ca *certificates.CA, pod corev1.Pod, nodeCertReconcileBefore time.Duration) bool {
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

	if time.Now().After(cert.NotAfter.Add(-nodeCertReconcileBefore)) {
		log.Info("Certificate soon to expire, should issue new", "secret", secret.Name)
		return true
	}

	// TODO: verify expected SANs in certificate, otherwise we wont actually reconcile such changes

	return false
}
