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
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.KBLog.WithName("nodecerts")

func ReconcileNodeCertificateSecrets(
	c k8s.Client,
	ca *certificates.Ca,
	csrClient certificates.CSRClient,
	es v1alpha1.ElasticsearchCluster,
	services []corev1.Service,
	trustRelationships []v1alpha1.TrustRelationship,
) (reconcile.Result, error) {
	log.Info("Reconciling node certificate secrets")

	additionalCAs := make([][]byte, 0, len(trustRelationships))
	for _, trustRelationship := range trustRelationships {
		if trustRelationship.Spec.CaCert == "" {
			continue
		}

		additionalCAs = append(additionalCAs, []byte(trustRelationship.Spec.CaCert))
	}

	nodeCertificateSecrets, err := findNodeCertificateSecrets(c, es)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, secret := range nodeCertificateSecrets {
		// todo: error checking if label does not exist
		podName := secret.Labels[LabelAssociatedPod]

		var pod corev1.Pod
		if err := c.Get(types.NamespacedName{Namespace: secret.Namespace, Name: podName}, &pod); err != nil {
			if !apierrors.IsNotFound(err) {
				return reconcile.Result{}, err
			}

			// give some leniency in pods showing up only after a while.
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
				c, secret, pod, csrClient, es.Name, es.Namespace, services, ca, additionalCAs,
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

func findNodeCertificateSecrets(
	c k8s.Client,
	es v1alpha1.ElasticsearchCluster,
) ([]corev1.Secret, error) {
	var nodeCertificateSecrets corev1.SecretList
	listOptions := client.ListOptions{
		Namespace: es.Namespace,
		LabelSelector: labels.Set(map[string]string{
			label.ClusterNameLabelName: es.Name,
			LabelSecretUsage:           LabelSecretUsageNodeCertificates,
		}).AsSelector(),
	}

	if err := c.List(&listOptions, &nodeCertificateSecrets); err != nil {
		return nil, err
	}

	return nodeCertificateSecrets.Items, nil
}

// doReconcile ensures that the node certificate secret has the available and correct Data keys
// provided.
func doReconcile(
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
