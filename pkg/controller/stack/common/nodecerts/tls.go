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

	"github.com/elastic/stack-operators/pkg/controller/stack/common/nodecerts/certutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("node-certs")
)

const (
	// LabelAssociatedPod is a label key that indicates the resource is supposed to have a named associated pod
	LabelAssociatedPod = "nodecerts.stack.k8s.elastic.co/associated-pod"

	// LabelSecretUsage is a label key that specifies what the secret is used for
	LabelSecretUsage                 = "nodecerts.stack.k8s.elastic.co/secret-usage"
	// LabelSecretUsageNodeCertificates is the LabelSecretUsage value used for node certificates
	LabelSecretUsageNodeCertificates = "node-certificates"

	// LabelNodeCertificateType is a label key indicating what the node-certificates secret is used for
	LabelNodeCertificateType                 = "nodecerts.stack.k8s.elastic.co/node-certificate-type"
	// LabelNodeCertificateTypeElasticsearchAll is the LabelNodeCertificateType value used for Elasticsearch
	LabelNodeCertificateTypeElasticsearchAll = "elasticsearch.all"
)

// NodeCertificateSecretObjectKeyForPod returns the object key for the secret containing the node certificates for
// a given pod.
func NodeCertificateSecretObjectKeyForPod(pod corev1.Pod) types.NamespacedName {
	// TODO: trim and suffix?
	return types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}
}

// EnsureNodeCertificateSecretExists ensures that the secret containing the node certificate is present in the
// apiserver
func EnsureNodeCertificateSecretExists(
	c client.Client,
	scheme *runtime.Scheme,
	s deploymentsv1alpha1.Stack,
	pod corev1.Pod,
	nodeCertificateType string,
) error {
	secretObjectKey := NodeCertificateSecretObjectKeyForPod(pod)

	var secret corev1.Secret
	if err := c.Get(context.TODO(), secretObjectKey, &secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if apierrors.IsNotFound(err) {
		secret = corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      secretObjectKey.Name,
				Namespace: secretObjectKey.Namespace,

				Labels: map[string]string{
					LabelAssociatedPod:       pod.Name,
					LabelSecretUsage:         LabelSecretUsageNodeCertificates,
					LabelNodeCertificateType: nodeCertificateType,
				},
			},
		}

		if err := controllerutil.SetControllerReference(&s, &secret, scheme); err != nil {
			return err
		}

		if err := c.Create(context.TODO(), &secret); err != nil {
			return err
		}
	}

	return nil
}

// ReconcileNodeCertificateSecret ensures that
func ReconcileNodeCertificateSecret(
	s deploymentsv1alpha1.Stack,
	pod corev1.Pod,
	secret corev1.Secret,
	ca *Ca,
	c client.Client,
) (reconcile.Result, error) {
	// TODO: method should not generate the private key
	// TODO: method should take a CSR argument instead of creating it

	// a placeholder secret may have a nil secret.Data, so create it if it does not exist
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	// XXX: be a little crazy, live a little. push private keys over the network.
	if _, ok := secret.Data["node.key"]; !ok {
		key, err := rsa.GenerateKey(cryptorand.Reader, 2048)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "unable to generate the private key")
		}

		pemKeyBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})

		cr := x509.CertificateRequest{}
		csrBytes, err := x509.CreateCertificateRequest(cryptorand.Reader, &cr, key)
		if err != nil {
			return reconcile.Result{}, err
		}

		csr, err := x509.ParseCertificateRequest(csrBytes)
		if err != nil {
			return reconcile.Result{}, err
		}

		validatedCertificateTemplate, err := createValidatedCertificateTemplate(s, pod, csr)
		if err != nil {
			return reconcile.Result{}, err
		}

		certData, err := ca.CreateCertificateForValidatedCertificateTemplate(*validatedCertificateTemplate)
		if err != nil {
			return reconcile.Result{}, err
		}

		secret.Data["node.key"] = pemKeyBytes
		secret.Data["ca.pem"] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Cert.Raw})
		secret.Data["cert.pem"] = append(
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certData}),
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Cert.Raw})...,
		)

		if err := c.Update(context.TODO(), &secret); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// createValidatedCertificateTemplate validates a CSR and creates a certificate template
func createValidatedCertificateTemplate(
	s deploymentsv1alpha1.Stack,
	pod corev1.Pod,
	csr *x509.CertificateRequest,
) (*x509.Certificate, error) {
	commonName := fmt.Sprintf("%s.node.%s.es.%s.namespace.local", pod.Name, s.Name, s.Namespace)
	commonNameUTF8OtherName := &cryptutil.UTF8StringValuedOtherName{
		OID:   cryptutil.CommonNameObjectIdentifier,
		Value: commonName,
	}
	commonNameOtherName, err := commonNameUTF8OtherName.ToOtherName()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create othername")
	}

	generalNames := []cryptutil.GeneralName{{OtherName: *commonNameOtherName}}
	generalNamesBytes, err := cryptutil.MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		return nil, err
	}

	// TODO: csr signature is not checked, common name not verified
	// TODO: add services dns entries / ip addresses to cert?
	// TODO: add pod ip when it's available (e.g when we're doing this for real)

	certificateTemplate := x509.Certificate{
		Subject: pkix.Name{
			CommonName:         commonName,
			OrganizationalUnit: []string{s.Name},
		},

		DNSNames: []string{
			commonName,
			pod.Name,
		},
		IPAddresses: []net.IP{
			net.ParseIP(pod.Status.PodIP),
			net.ParseIP("127.0.0.1"),
		},

		ExtraExtensions: []pkix.Extension{
			{Id: cryptutil.SubjectAlternativeNamesObjectIdentifier, Value: generalNamesBytes},
		},
		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		Signature:          csr.Signature,
		SignatureAlgorithm: csr.SignatureAlgorithm,

		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	return &certificateTemplate, nil
}
