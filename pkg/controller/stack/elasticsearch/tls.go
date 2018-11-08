package elasticsearch

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/certutil"
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
	SerialNumberLimit = new(big.Int).Lsh(big.NewInt(1), 128)

	log = logf.Log.WithName("stack-controller")
)

const (
	LabelBelongsToPod                = "elasticsearch.k8s.elastic.co/belongs-to-pod"
	LabelSecretUsage                 = "elasticsearch.k8s.elastic.co/secret-usage"
	LabelSecretUsageNodeCertificates = "node-certificates"
)

// final intended workflow
// 1. create placeholder secret for node cert + ca
// 2. pod creates CSR-like, pushes to api server
// 3. validate csr originates from pod (TODO: how?)
// 4. issue certificate based on csr, fill in placeholder secret
// 5. whenever our basis for the issued cert changes, update placeholder secret

// 1. create placeholder secret for node cert + ca
// 2. cant wait for a csr, so pretend we have one..
// 3. issue certificate based on csr
// 3. fill in placeholder secret with node cert + ca + private keys (ugh)

func NodeCerificateSecretObjectKeyForPod(pod corev1.Pod) types.NamespacedName {
	return types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}
}

func EnsureNodeCertificateSecretExists(
	c client.Client,
	scheme *runtime.Scheme,
	s deploymentsv1alpha1.Stack,
	pod corev1.Pod,
) error {
	secretObjectKey := NodeCerificateSecretObjectKeyForPod(pod)

	var secret corev1.Secret
	if err := c.Get(context.TODO(), secretObjectKey, &secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if apierrors.IsNotFound(err) {
		secret = corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      secretObjectKey.Name,
				Namespace: secretObjectKey.Namespace,

				Labels: map[string]string{
					LabelBelongsToPod: pod.Name,
					LabelSecretUsage:  LabelSecretUsageNodeCertificates,
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

func ReconcileNodeCertificateSecret(
	s deploymentsv1alpha1.Stack,
	pod corev1.Pod,
	secret corev1.Secret,
	caCert *x509.Certificate,
	caKeys *rsa.PrivateKey,
	c client.Client,
) (reconcile.Result, error) {
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	// be a little crazy, live a little. push private keys over the network.
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

		certData, err := issueCertificate(caCert, caKeys, s, pod, csr)
		if err != nil {
			return reconcile.Result{}, err
		}

		secret.Data["node.key"] = pemKeyBytes
		secret.Data["ca.pem"] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
		secret.Data["cert.pem"] = append(
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certData}),
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})...,
		)

		if err := c.Update(context.TODO(), &secret); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func issueCertificate(
	caCert *x509.Certificate,
	caKeys *rsa.PrivateKey,
	s deploymentsv1alpha1.Stack,
	pod corev1.Pod,
	csr *x509.CertificateRequest,
) ([]byte, error) {
	// generate a serial number
	serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate serial number for new certificate")
	}

	commonName := fmt.Sprintf("%s.node.%s.cluster.local", pod.Name, s.Name)
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
		panic(err)
	}

	// TODO: csr signature is not checked, common name not verified
	// TODO: add services dns entries / ip addresses to cert?
	// TODO: add pod ip when it's available (e.g when we're doing this for real)

	certificateTemplate := x509.Certificate{
		SerialNumber: serial,
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

		Issuer: caCert.Subject,

		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	certData, err := x509.CreateCertificate(cryptorand.Reader, &certificateTemplate, caCert, csr.PublicKey, caKeys)

	return certData, err
}
