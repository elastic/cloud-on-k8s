// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package main

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"io/ioutil"
	"os"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func tmpConfig() Config {
	privateKeyTmpFile, err := ioutil.TempFile("", "private.key")
	exitOnErr(err)
	csrTmpFile, err := ioutil.TempFile("", "csr")
	exitOnErr(err)
	certTmpFile, err := ioutil.TempFile("", "cert")
	exitOnErr(err)
	config := Config{
		PrivateKeyPath: privateKeyTmpFile.Name(),
		CSRPath:        csrTmpFile.Name(),
		CertPath:       certTmpFile.Name(),
	}
	return config
}

func cleanTmpConfig(config Config) {
	os.Remove(config.PrivateKeyPath)
	os.Remove(config.CSRPath)
	os.Remove(config.CertPath)
}

func createAndStoreCert(csrBytes []byte, path string) error {
	csr, err := x509.ParseCertificateRequest(csrBytes)
	if err != nil {
		return err
	}
	ca, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	if err != nil {
		return err
	}
	pod := corev1.Pod{
		Status: corev1.PodStatus{
			PodIP: "172.18.1.1",
		},
	}
	clusterName := "clustername"
	namespace := "namespace"
	svcs := []corev1.Service{}
	validatedCertificateTemplate, err := nodecerts.CreateValidatedCertificateTemplate(pod, clusterName, namespace, svcs, csr, certificates.DefaultCertValidity)
	if err != nil {
		return err
	}
	certData, err := ca.CreateCertificate(*validatedCertificateTemplate)
	if err != nil {
		return err
	}
	asPem := certificates.EncodePEMCert(certData)
	return ioutil.WriteFile(path, asPem, 644)
}

func createValidFiles(config Config) error {
	// create and store private key
	privateKey, err := createAndStorePrivateKey(config.PrivateKeyPath)
	if err != nil {
		return err
	}
	// create and store csr
	csr, err := createCSR(privateKey)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(config.CSRPath, csr, 644); err != nil {
		return err
	}
	// create and store cert
	if err := createAndStoreCert(csr, config.CertPath); err != nil {
		return err
	}
	return nil
}

func Test_checkExistingOnDisk(t *testing.T) {
	tests := []struct {
		name      string
		runBefore func(config Config)
		want      bool
	}{
		{
			name:      "private key file does not exist",
			runBefore: func(config Config) {},
			want:      false,
		},
		{
			name: "private key file is invalid",
			runBefore: func(config Config) {
				err := ioutil.WriteFile(config.PrivateKeyPath, []byte("invalid key"), 644)
				require.NoError(t, err)
			},
			want: false,
		},
		{
			name: "csr file does not exist",
			runBefore: func(config Config) {
				// create and store private key
				_, err := createAndStorePrivateKey(config.PrivateKeyPath)
				require.NoError(t, err)
			},
			want: false,
		},
		{
			name: "csr file is invalid",
			runBefore: func(config Config) {
				// create and store private key
				_, err := createAndStorePrivateKey(config.PrivateKeyPath)
				require.NoError(t, err)
				// write invalid csr
				err = ioutil.WriteFile(config.CSRPath, []byte("invalid csr"), 644)
				require.NoError(t, err)
			},
			want: false,
		},
		{
			name: "cert file does not exist",
			runBefore: func(config Config) {
				// create and store private key
				privateKey, err := createAndStorePrivateKey(config.PrivateKeyPath)
				require.NoError(t, err)
				// create and store csr
				csr, err := createCSR(privateKey)
				require.NoError(t, err)
				err = ioutil.WriteFile(config.CSRPath, csr, 644)
				require.NoError(t, err)
			},
			want: false,
		},
		{
			name: "cert file is invalid",
			runBefore: func(config Config) {
				// create and store private key
				privateKey, err := createAndStorePrivateKey(config.PrivateKeyPath)
				require.NoError(t, err)
				// create and store csr
				csr, err := createCSR(privateKey)
				require.NoError(t, err)
				err = ioutil.WriteFile(config.CSRPath, csr, 644)
				require.NoError(t, err)
				// write invalid cert
				err = ioutil.WriteFile(config.CertPath, []byte("invalid cert"), 644)
				require.NoError(t, err)
			},
			want: false,
		},
		{
			name: "private key, csr and cert can be reused",
			runBefore: func(config Config) {
				createValidFiles(config)
			},
			want: true,
		},
		{
			name: "private key and csr do not match",
			runBefore: func(config Config) {
				// create and store valid files
				createValidFiles(config)
				// replace CSR by another one generated from a different private key
				privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				csr, err := createCSR(privateKey2)
				require.NoError(t, err)
				err = ioutil.WriteFile(config.CSRPath, csr, 644)
				require.NoError(t, err)
			},
			want: false,
		},
		{
			name: "private key and cert do not match",
			runBefore: func(config Config) {
				// create and store valid files
				err := createValidFiles(config)
				require.NoError(t, err)
				// replace cert by another one generated from a different private key
				privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				csr2, err := createCSR(privateKey2)
				require.NoError(t, err)
				err = createAndStoreCert(csr2, config.CertPath)
				require.NoError(t, err)
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tmpConfig()
			defer cleanTmpConfig(config)
			tt.runBefore(config)
			assert.Equal(t, tt.want, checkExistingOnDisk(config))
		})
	}
}
func Test_watchForCertUpdate(t *testing.T) {
	config := tmpConfig()
	defer cleanTmpConfig(config)
	done := make(chan struct{})

	certInit := NewCertInitializer(config)

	// watch in background
	go func() {
		err := certInit.watchForCertUpdate()
		require.NoError(t, err)
		close(done)
	}()
	// write a valid cert
	err := createValidFiles(config)
	require.NoError(t, err)
	// we should be done before unit tests timeout
	<-done
}
