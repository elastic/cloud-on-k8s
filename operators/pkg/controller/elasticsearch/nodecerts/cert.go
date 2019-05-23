// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
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

	// load additional trusted CAs from the trustrelationships
	additionalCAs := make([][]byte, 0, len(trustRelationships))
	for _, trustRelationship := range trustRelationships {
		if trustRelationship.Spec.CaCert == "" {
			continue
		}

		additionalCAs = append(additionalCAs, []byte(trustRelationship.Spec.CaCert))
	}

	// build the trust.yml file
	trustRootCfg := NewTrustRootConfig(es.Name, es.Namespace)

	// include the trust restrictions from the trust relationships into the trust restrictions config
	for _, trustRelationship := range trustRelationships {
		trustRootCfg.Include(trustRelationship.Spec.TrustRestrictions)
	}

	trustRootCfgData, err := json.Marshal(&trustRootCfg)
	if err != nil {
		return reconcile.Result{}, err
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
				c, secret, pod, csrClient, es, services, ca, additionalCAs, trustRootCfgData, nodeCertValidity, nodeCertRotateBefore,
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
	cluster v1alpha1.Elasticsearch,
	svcs []corev1.Service,
	ca *certificates.CA,
	additionalTrustedCAsPemEncoded [][]byte,
	trustRootCfgData []byte,
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
	issueNewCertificate := shouldIssueNewCertificate(cluster, svcs, secret, ca, pod, nodeCertReconcileBefore)

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

	if issueNewCertificate {
		log.Info(
			"Issuing new certificate",
			"secret", secret.Name,
			"clusterName", cluster.Name,
			"namespace", cluster.Namespace,
		)

		// create a cert from the csr
		parsedCSR, err := x509.ParseCertificateRequest(csr)
		if err != nil {
			return reconcile.Result{}, err
		}
		validatedCertificateTemplate, err := CreateValidatedCertificateTemplate(pod, cluster, svcs, parsedCSR, nodeCertValidity)
		if err != nil {
			return reconcile.Result{}, err
		}
		// sign the certificate
		certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
		if err != nil {
			return reconcile.Result{}, err
		}

		// store CSR and signed certificate in a secret mounted into the pod
		secret.Data[CSRFileName] = csr
		secret.Data[CertFileName] = certificates.EncodePEMCert(certData, ca.Cert.Raw)
		// store last CSR update in the pod annotations
		secret.Annotations[LastCSRUpdateAnnotation] = lastCSRUpdate
	}

	// prepare trusted CA certs: CA of this node + additional CA certs from trustrelationships
	trusted := certificates.EncodePEMCert(ca.Cert.Raw)
	for _, caPemBytes := range additionalTrustedCAsPemEncoded {
		trusted = append(trusted, caPemBytes...)
	}

	// compare with current trusted CA certs.
	updateTrustedCACerts := !bytes.Equal(trusted, secret.Data[certificates.CAFileName])
	if updateTrustedCACerts {
		secret.Data[certificates.CAFileName] = trusted
	}

	updateTrustRestrictions := !bytes.Equal(trustRootCfgData, secret.Data[TrustRestrictionsFilename])
	if updateTrustRestrictions {
		secret.Data[TrustRestrictionsFilename] = trustRootCfgData
	}

	if issueNewCertificate || updateTrustedCACerts || updateTrustRestrictions {
		log.Info("Updating node certificate secret", "secret", secret.Name)
		if err := c.Update(&secret); err != nil {
			return reconcile.Result{}, err
		}
		annotation.MarkPodAsUpdated(c, pod)
	}

	return reconcile.Result{}, nil
}

func extractNodeCert(secret corev1.Secret, commonName string) *x509.Certificate {
	certData, ok := secret.Data[CertFileName]
	if !ok {
		return nil
	}

	certs, err := certificates.ParsePEMCerts(certData)
	if err != nil {
		log.Error(err, "Invalid certificate data found, issuing new certificate", "secret", secret.Name)
		return nil
	}

	// look for the node certificate
	for _, c := range certs {
		if c.Subject.CommonName == commonName {
			return c
		}
	}

	return nil
}

// shouldIssueNewCertificate returns true if we should issue a new certificate.
// Reasons for reissuing a certificate:
// - no certificate yet
// - certificate has the wrong format
// - certificate is invalid or expired
// - certificate SAN and IP does not match pod SAN and IP
func shouldIssueNewCertificate(cluster v1alpha1.Elasticsearch, svcs []corev1.Service, secret corev1.Secret, ca *certificates.CA, pod corev1.Pod, nodeCertReconcileBefore time.Duration) bool {
	certCommonName := buildCertificateCommonName(pod, cluster.Name, cluster.Namespace)
	cert := extractNodeCert(secret, certCommonName)
	if cert == nil {
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

	// compare actual vs. expected SANs
	expected, err := createSubjectAltNameExt(cluster, svcs, pod)
	if err != nil {
		log.Error(err, "Cannot create subject alternative names", "pod", pod.Name)
		return true
	}
	extraExtensionFound := false
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(certificates.SubjectAlternativeNamesObjectIdentifier) {
			continue
		}
		extraExtensionFound = true
		if !reflect.DeepEqual(ext.Value, expected) {
			log.Info("Certificate SANs do not match expected one, should issue new", "secret", secret.Name)
			return true
		}
	}
	if !extraExtensionFound {
		log.Info("SAN extra extension not found, should issue new certificate", "secret", secret.Name)
		return true
	}

	return false
}
