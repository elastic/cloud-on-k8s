// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/elastic/cloud-on-k8s/pkg/utils/cryptutil"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// CheckTransportCACertificate attempts a TLS handshake to inspect the peer certificates presented by the Elasticsearch
// node to verify the expected CA certificate is among them.
func CheckTransportCACertificate(es esv1.Elasticsearch, ca *x509.Certificate) error {
	host := services.ExternalTransportServiceHost(k8s.ExtractNamespacedName(&es))
	var conn net.Conn
	var err error

	if test.Ctx().AutoPortForwarding {
		conn, err = portforward.NewForwardingDialer().DialContext(context.Background(), "tcp", host)
	} else {
		conn, err = net.Dial("tcp", host)
	}
	if err != nil {
		return err
	}

	defer conn.Close()

	certPool := x509.NewCertPool()
	certPool.AddCert(ca)
	config := tls.Config{
		RootCAs: certPool,
		// go requires either ServerName or InsecureSkipVerify (or both) when handshaking as a client since 1.3:
		// https://github.com/golang/go/commit/fca335e91a915b6aae536936a7694c4a2a007a60
		InsecureSkipVerify: true, // nolint:gosec
	}
	var correctCertsPresented bool
	config.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		// we are not interested in a valid TLS handshake but only in the CA certs presented by the remote side
		// therefore we only verify the peer certificate chain against our expected CA cert. We cannot rely on
		// tls.ConnectionState because it is only populated with the peer certificates after a successful handshake
		_, _, err := cryptutil.VerifyCertificateExceptServerName(rawCerts, &config)
		if err == nil {
			correctCertsPresented = true
		}
		return err
	}
	client := tls.Client(conn, &config)
	// handshake can fail on 6.x versions of Elasticsearch because the test client is not presenting the right certificates
	// but we are only interested in the peer certificates
	err = client.Handshake()
	if correctCertsPresented {
		return nil
	}
	return fmt.Errorf("expected %v %s among peer certificates but was not found, handshake err %w", ca.Issuer, ca.SerialNumber, err)
}

func (b Builder) CheckTransportCertificatesStep(k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Verify TLS CA cert on transport layer is the expected one",
		Test: test.Eventually(func() error {
			var secret corev1.Secret
			secretName := certificates.PublicTransportCertsSecretName(esv1.ESNamer, b.Elasticsearch.Name)
			secretNSN := types.NamespacedName{
				Namespace: b.Elasticsearch.Namespace,
				Name:      secretName,
			}
			if err := k.Client.Get(context.Background(), secretNSN, &secret); err != nil {
				return err
			}
			caCertsData, exists := secret.Data[certificates.CAFileName]
			if !exists {
				return fmt.Errorf("no %s found for cert in secret %s", certificates.CAFileName, secretName)
			}
			caCerts, err := certificates.ParsePEMCerts(caCertsData)
			if err != nil {
				return err
			}
			if len(caCerts) != 1 {
				return fmt.Errorf("expected exactly one CA certificate inside %s in %s but found %v",
					certificates.CAFileName, secretName, len(caCerts),
				)
			}
			return CheckTransportCACertificate(b.Elasticsearch, caCerts[0])
		}),
	}
}
