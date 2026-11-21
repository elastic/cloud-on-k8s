// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build logstash || e2e

package logstash

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/logstash"
)

const (
	beatsServiceName = "beats"
	beatsPort        = 5044
	// beatsExternalSAN is a name which is not one of the names of the Kubernetes Services of Logstash.
	beatsExternalSAN = "logstash-external.example.com"
)

// TestLogstashCustomServiceTLS verifies that the subject alternative names of a custom Service are added to the
// certificate shared by the Services of Logstash, and that a beats input can use this certificate and its key from
// the directory where the operator mounts them.
func TestLogstashCustomServiceTLS(t *testing.T) {
	b := newBeatsTLSBuilder("test-ls-svc-tls")

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			checkBeatsCertificate(b, k, beatsExternalSAN),
			checkBeatsCertificate(b, k, beatsServiceDNSName(b)),
		}
	})

	test.Sequence(nil, steps, b).RunSequential(t)
}

// TestLogstashCustomServiceTLSWithAPITLSDisabled verifies that the certificate is issued and mounted for a
// custom Service when TLS is disabled for the API Service.
func TestLogstashCustomServiceTLSWithAPITLSDisabled(t *testing.T) {
	b := newBeatsTLSBuilder("test-ls-svc-tls-noapi",
		logstashv1alpha1.LogstashService{
			Name: "api",
			TLS: commonv1.TLSOptions{
				SelfSignedCertificate: &commonv1.SelfSignedCertificate{
					Disabled: true,
				},
			},
		},
	)

	steps := test.StepsFunc(func(k *test.K8sClient) test.StepList {
		return test.StepList{
			checkBeatsCertificate(b, k, beatsExternalSAN),
			checkBeatsCertificate(b, k, beatsServiceDNSName(b)),
		}
	})

	test.Sequence(nil, steps, b).RunSequential(t)
}

// newBeatsTLSBuilder returns a Logstash with a beats input which uses the certificate managed by the operator, and
// a custom Service for this input which adds a subject alternative name to this certificate.
func newBeatsTLSBuilder(name string, extraServices ...logstashv1alpha1.LogstashService) logstash.Builder {
	certsDir := certificates.HTTPCertificatesSecretVolumeMountPath
	beatsService := logstashv1alpha1.LogstashService{
		Name: beatsServiceName,
		Service: commonv1.ServiceTemplate{
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: beatsServiceName, Port: beatsPort, Protocol: corev1.ProtocolTCP},
				},
			},
		},
		TLS: commonv1.TLSOptions{
			SelfSignedCertificate: &commonv1.SelfSignedCertificate{
				SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
					{DNS: beatsExternalSAN},
				},
			},
		},
	}

	return logstash.NewBuilder(name).
		WithNodeCount(1).
		WithPipelines([]commonv1.Config{
			{
				Data: map[string]interface{}{
					"pipeline.id": "main",
					"config.string": fmt.Sprintf(
						`input { beats { port => %d ssl_enabled => true ssl_certificate => "%s/%s" ssl_key => "%s/%s" } } output { stdout{} }`,
						beatsPort, certsDir, certificates.CertFileName, certsDir, certificates.KeyFileName,
					),
				},
			},
		}).
		WithServices(append(extraServices, beatsService)...)
}

func beatsServiceDNSName(b logstash.Builder) string {
	return fmt.Sprintf("%s.%s.svc", logstashv1alpha1.UserServiceName(b.Logstash.Name, beatsServiceName), b.Logstash.Namespace)
}

// checkBeatsCertificate opens a TLS connection to the beats input and verifies the certificate of the input against
// the CA published by the operator, for the given server name.
func checkBeatsCertificate(b logstash.Builder, k *test.K8sClient, serverName string) test.Step {
	return test.Step{
		Name: fmt.Sprintf("Certificate of the beats input should be valid for %s", serverName),
		Test: test.Eventually(func() error {
			// the public certificates Secret holds the certificate chain, which includes the CA
			certs, err := k.GetHTTPCerts(logstashv1alpha1.Namer, b.Logstash.Namespace, b.Logstash.Name)
			if err != nil {
				return fmt.Errorf("while getting the public HTTP certificates of Logstash: %w", err)
			}
			rootCAs := x509.NewCertPool()
			for _, cert := range certs {
				if cert.IsCA {
					rootCAs.AddCert(cert)
				}
			}

			addr := net.JoinHostPort(beatsServiceDNSName(b), strconv.Itoa(beatsPort))
			var conn net.Conn
			if test.Ctx().AutoPortForwarding {
				conn, err = portforward.NewForwardingDialer().DialContext(context.Background(), "tcp", addr)
			} else {
				conn, err = net.Dial("tcp", addr) //nolint:noctx
			}
			if err != nil {
				return fmt.Errorf("dial to Logstash beats input: %w", err)
			}
			defer conn.Close()

			// the hostname verification is done against serverName, not against the address used to connect
			tlsConn := tls.Client(conn, &tls.Config{
				RootCAs:    rootCAs,
				ServerName: serverName,
				MinVersion: tls.VersionTLS12,
			})
			if err := tlsConn.Handshake(); err != nil {
				return fmt.Errorf("TLS handshake with Logstash beats input for server name %s: %w", serverName, err)
			}
			return nil
		}),
	}
}
