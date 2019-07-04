// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"reflect"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("transport")

// ReconcileTransportCertificateSecrets reconciles certificate secrets for nodes
// of the given es cluster.
func ReconcileTransportCertificateSecrets(
	c k8s.Client,
	scheme *runtime.Scheme,
	ca *certificates.CA,
	es v1alpha1.Elasticsearch,
	services []corev1.Service,
	trustRelationships []v1alpha1.TrustRelationship,
	rotationParams certificates.RotationParams,
) (reconcile.Result, error) {
	log.Info("Reconciling transport certificate secrets", "namespace", es.Namespace, "es_name", es.Name)

	// load additional trusted CAs from the trustrelationships
	additionalCAs := make([][]byte, 0, len(trustRelationships))
	for _, trustRelationship := range trustRelationships {
		if trustRelationship.Spec.CaCert == "" {
			continue
		}

		additionalCAs = append(additionalCAs, []byte(trustRelationship.Spec.CaCert))
	}

	var pods corev1.PodList
	if err := c.List(&client.ListOptions{
		LabelSelector: label.NewLabelSelectorForElasticsearch(es),
		Namespace:     es.Namespace,
	}, &pods); err != nil {
		return reconcile.Result{}, err
	}

	for _, pod := range pods.Items {
		if pod.Status.PodIP == "" {
			log.Info("Skipping pod because it has no IP yet", "namespace", pod.Namespace, "pod_name", pod.Name)
			continue
		}

		if res, err := doReconcileTransportCertificateSecret(
			c, scheme, es, pod, services, ca, additionalCAs, rotationParams,
		); err != nil {
			return res, err
		}
	}

	return reconcile.Result{}, nil
}

// doReconcileTransportCertificateSecret ensures that the transport certificate secret has the correct content.
func doReconcileTransportCertificateSecret(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	pod corev1.Pod,
	svcs []corev1.Service,
	ca *certificates.CA,
	additionalTrustedCAsPemEncoded [][]byte,
	rotationParams certificates.RotationParams,
) (reconcile.Result, error) {
	secret, err := EnsureTransportCertificateSecretExists(c, scheme, es, pod)
	if err != nil {
		return reconcile.Result{}, err
	}

	// a placeholder secret may have nil entries, create them if needed
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}

	// verify that the secret contains a parsable private key, create if it does not exist
	var privateKey *rsa.PrivateKey
	needsNewPrivateKey := true
	if privateKeyData, ok := secret.Data[certificates.KeyFileName]; ok {
		storedPrivateKey, err := certificates.ParsePEMPrivateKey(privateKeyData)
		if err != nil {
			log.Error(err, "Unable to parse stored private key", "namespace", secret.Namespace, "secret_name", secret.Name)
		} else {
			needsNewPrivateKey = false
			privateKey = storedPrivateKey
		}
	}

	// if we need a new private key, generate it
	if needsNewPrivateKey {
		generatedPrivateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return reconcile.Result{}, err
		}

		privateKey = generatedPrivateKey
		secret.Data[certificates.KeyFileName] = certificates.EncodePEMPrivateKey(*privateKey)
	}

	// check if the existing cert is correct
	issueNewCertificate := shouldIssueNewCertificate(es, svcs, *secret, privateKey, ca, pod, rotationParams.RotateBefore)

	if issueNewCertificate {
		log.Info(
			"Issuing new certificate",
			"namespace", pod.Namespace,
			"pod_name", pod.Name,
			"es_name", es.Name,
		)

		csr, err := x509.CreateCertificateRequest(cryptorand.Reader, &x509.CertificateRequest{}, privateKey)
		if err != nil {
			return reconcile.Result{}, err
		}

		// create a cert from the csr
		parsedCSR, err := x509.ParseCertificateRequest(csr)
		if err != nil {
			return reconcile.Result{}, err
		}

		validatedCertificateTemplate, err := CreateValidatedCertificateTemplate(pod, es, svcs, parsedCSR, rotationParams.Validity)
		if err != nil {
			return reconcile.Result{}, err
		}
		// sign the certificate
		certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
		if err != nil {
			return reconcile.Result{}, err
		}

		// store the issued certificate in a secret mounted into the pod
		secret.Data[certificates.CertFileName] = certificates.EncodePEMCert(certData, ca.Cert.Raw)
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

	if needsNewPrivateKey || issueNewCertificate || updateTrustedCACerts {
		log.Info("Updating transport certificate secret", "namespace", secret.Namespace, "secret_name", secret.Name, "es_name", es.Name, "pod_name", pod.Name)
		if err := c.Update(secret); err != nil {
			return reconcile.Result{}, err
		}
		annotation.MarkPodAsUpdated(c, pod)
	}

	return reconcile.Result{}, nil
}

// extractTransportCert extracts the transport certificate with the commonName from the Secret
func extractTransportCert(secret corev1.Secret, commonName string) *x509.Certificate {
	certData, ok := secret.Data[certificates.CertFileName]
	if !ok {
		log.Info("No tls certificate found in secret", "namespace", secret.Namespace, "secret, name", secret.Name)
		return nil
	}

	certs, err := certificates.ParsePEMCerts(certData)
	if err != nil {
		log.Error(err, "Invalid certificate data found, issuing new certificate", "namespace", secret.Namespace, "secret_name", secret.Name)
		return nil
	}

	// look for the certificate based on the CommonName
	var names []string
	for _, c := range certs {
		if c.Subject.CommonName == commonName {
			return c
		}
		names = append(names, c.Subject.CommonName)
	}

	log.Info("Did not find a certificate with the expected common name", "namespace", secret.Namespace,
		"secret_name", secret.Name, "expected", commonName, "actual", names)

	return nil
}

// shouldIssueNewCertificate returns true if we should issue a new certificate.
//
// Reasons for reissuing a certificate:
// - no certificate yet
// - certificate has the wrong format
// - certificate is invalid or expired
// - certificate SAN and IP does not match pod SAN and IP
func shouldIssueNewCertificate(
	cluster v1alpha1.Elasticsearch,
	svcs []corev1.Service,
	secret corev1.Secret,
	privateKey *rsa.PrivateKey,
	ca *certificates.CA,
	pod corev1.Pod,
	certReconcileBefore time.Duration,
) bool {
	certCommonName := buildCertificateCommonName(pod, cluster.Name, cluster.Namespace)

	generalNames, err := buildGeneralNames(cluster, svcs, pod)
	if err != nil {
		log.Error(err, "Cannot create GeneralNames for the TLS certificate", "namespace", pod.Namespace, "pod_name", pod.Name, "es_name", cluster.Name)
		return true
	}

	cert := extractTransportCert(secret, certCommonName)
	if cert == nil {
		return true
	}

	publicKey, publicKeyOk := cert.PublicKey.(*rsa.PublicKey)
	if !publicKeyOk || publicKey.N.Cmp(privateKey.PublicKey.N) != 0 || publicKey.E != privateKey.PublicKey.E {
		log.Info(
			"Certificate belongs to a different public key, should issue new",
			"subject", cert.Subject,
			"issuer", cert.Issuer,
			"current_ca_subject", ca.Cert.Subject,
			"secret_name", secret.Name,
			"namespace", secret.Namespace,
			"es_name", cluster.Name,
		)
		return true
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	verifyOpts := x509.VerifyOptions{
		DNSName:       certCommonName,
		Roots:         pool,
		Intermediates: pool,
	}
	if _, err := cert.Verify(verifyOpts); err != nil {
		// this is not necessarily an error because the certificate may be expired
		log.Info(
			fmt.Sprintf("Certificate was not valid, should issue new: %s", err),
			"subject", cert.Subject,
			"issuer", cert.Issuer,
			"current_ca_subject", ca.Cert.Subject,
			"namespace", secret.Namespace,
			"secret_name", secret.Name,
		)
		return true
	}

	if time.Now().After(cert.NotAfter.Add(-certReconcileBefore)) {
		log.Info("Certificate soon to expire, should issue new", "namespace", secret.Namespace, "secret_name", secret.Name)
		return true
	}

	// compare actual vs. expected SANs
	expected, err := certificates.MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		log.Error(err, "Cannot marshal subject alternative names", "namespace", secret.Namespace, "secret_name", secret.Name)
		return true
	}
	extraExtensionFound := false
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(certificates.SubjectAlternativeNamesObjectIdentifier) {
			continue
		}
		extraExtensionFound = true
		if !reflect.DeepEqual(ext.Value, expected) {
			log.Info("Certificate SANs do not match expected one, should issue new", "namespace", secret.Namespace, "secret_name", secret.Name)
			return true
		}
	}
	if !extraExtensionFound {
		log.Info("SAN extra extension not found, should issue new certificate", "namespace", secret.Namespace, "secret_name", secret.Name)
		return true
	}

	return false
}
