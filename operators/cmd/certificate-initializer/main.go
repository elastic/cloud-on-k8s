// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	certs "k8s.io/api/certificates/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	commonName         string
	additionalDNSNames string

	privateKeyPath string

	clusterDomain string

	namespace string
	podIP     string
	podName   string

	subdomain string
	hostname  string

	ownerReferenceJSON string
	annotationsJSON    string
	labelsJSON         string

	log = logf.Log.WithName("entrypoint")
)

func main() {
	logf.SetLogger(logf.ZapLogger(false))

	flag.StringVar(&namespace, "namespace", "default", "namespace as defined by pod.metadata.namespace")
	flag.StringVar(&podIP, "pod-ip", "", "IP address as defined by pod.status.podIP")
	flag.StringVar(&podName, "pod-name", "", "name as defined by pod.metadata.name")

	flag.StringVar(&commonName, "common-name", "", "common name for the CSR")
	flag.StringVar(&additionalDNSNames, "additional-dnsnames", "", "additional dns names; comma separated")

	flag.StringVar(&privateKeyPath, "private-key-path", "/etc/tls/tls.key", "The directory where the private key should be written")

	flag.StringVar(&clusterDomain, "cluster-domain", "cluster.local", "Kubernetes cluster domain")

	flag.StringVar(&hostname, "hostname", "", "hostname as defined by pod.spec.hostname")
	flag.StringVar(&subdomain, "subdomain", "", "subdomain as defined by pod.spec.subdomain")

	flag.StringVar(&ownerReferenceJSON, "csr-owner-reference", "", "owner reference for our CSR; as json")
	flag.StringVar(&annotationsJSON, "csr-annotations", "", "annotations for our CSR; as json")
	flag.StringVar(&labelsJSON, "csr-labels", "", "labels for our CSR; as json")
	flag.Parse()

	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "cannot create config to talk to apiserver")
		os.Exit(1)
	}

	client, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Error(err, "cannot create kubernetes client")
		os.Exit(1)
	}
	c := k8s.WrapClient(client)

	key, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		log.Error(err, "unable to generate the private key")
		os.Exit(1)
	}

	if err := storePrivateKey(key, privateKeyPath); err != nil {
		log.Error(err, "unable to write to %s", privateKeyPath)
		os.Exit(1)
	}
	log.Info(fmt.Sprintf("Wrote private key to %s", privateKeyPath))

	dnsNames := []string{commonName}
	dnsNames = append(dnsNames, buildDNSNames()...)

	certificateTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: commonName,
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
		DNSNames:           dnsNames,
		IPAddresses:        ipAddresses(),
	}

	csrBytes, err := x509.CreateCertificateRequest(cryptorand.Reader, certificateTemplate, key)
	if err != nil {
		log.Error(err, "unable to create certificate request")
		os.Exit(1)
	}

	csr := certs.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: certs.CertificateSigningRequestSpec{
			Request: csrBytes,
		},
	}

	if ownerReferenceJSON != "" {
		ownerRef := metav1.OwnerReference{}
		if err := json.Unmarshal([]byte(ownerReferenceJSON), &ownerRef); err != nil {
			log.Error(err, "unable to parse owner reference")
			os.Exit(1)
		}
		csr.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	}

	if annotationsJSON != "" {
		var annotations map[string]string
		if err := json.Unmarshal([]byte(annotationsJSON), &annotations); err != nil {
			log.Error(err, "unable to parse annotations")
			os.Exit(1)
		}
		csr.ObjectMeta.Annotations = annotations
	}

	if labelsJSON != "" {
		var labels map[string]string
		if err := json.Unmarshal([]byte(labelsJSON), &labels); err != nil {
			log.Error(err, "unable to parse labels")
			os.Exit(1)
		}
		csr.ObjectMeta.Labels = labels
	}

	if err := c.Create(&csr); err != nil && !apierrors.IsAlreadyExists(err) {
		log.Error(err, "unable to create CSR resource")
		os.Exit(1)
	}
}

func buildDNSNames() []string {
	// Gather a list of DNS names that resolve to this pod which include the
	// default DNS name:
	//   - ${pod-ip-address}.${namespace}.pod.${cluster-domain}
	//
	// A dns name will be added for each additional DNS name provided via the
	// `-additional-dnsnames` flag.
	dnsNames := defaultDNSNames(podIP, hostname, subdomain, namespace, clusterDomain)

	for _, n := range strings.Split(additionalDNSNames, ",") {
		if n == "" {
			continue
		}
		dnsNames = append(dnsNames, n)
	}

	return dnsNames
}

func defaultDNSNames(ip, hostname, subdomain, namespace, clusterDomain string) []string {
	ns := []string{podDomainName(ip, namespace, clusterDomain)}
	if hostname != "" && subdomain != "" {
		ns = append(ns, podHeadlessDomainName(hostname, subdomain, namespace, clusterDomain))
	}
	return ns
}

func podDomainName(ip, namespace, domain string) string {
	return fmt.Sprintf("%s.%s.pod.%s", strings.Replace(ip, ".", "-", -1), namespace, domain)
}

func podHeadlessDomainName(hostname, subdomain, namespace, domain string) string {
	if hostname == "" || subdomain == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s.svc.%s", hostname, subdomain, namespace, domain)
}

func ipAddresses() []net.IP {
	// Gather the list of IP addresses for the certificate's IP SANs field which
	// include:
	//   - the pod IP address
	//   - 127.0.0.1 for localhost access
	ip := net.ParseIP(podIP)
	if ip.To4() == nil && ip.To16() == nil {
		log.Error(nil, "invalid pod IP address")
		os.Exit(1)
	}

	ipaddresses := []net.IP{ip, net.ParseIP("127.0.0.1")}

	return ipaddresses
}

func storePrivateKey(key *rsa.PrivateKey, keyPath string) error {
	pemKeyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	if err := ioutil.WriteFile(keyPath, pemKeyBytes, 0644); err != nil {
		return err
	}

	return nil
}
