// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"context"
	"crypto/x509"
	"io/ioutil"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
)

const waitEsReadinessPeriod = 10 * time.Second

type EsClient interface {
	ReloadSecureSettings() error
	WaitForEsReady()
}

type esClient struct {
	CACertsPath string
	endpoint    string
	user        client.UserAuth
	version     version.Version
}

func NewEsClient(cfg Config) EsClient {
	return esClient{
		cfg.EsCACertsPath,
		cfg.EsEndpoint,
		cfg.EsUser,
		cfg.EsVersion,
	}
}

// reloadSecureSettings tries to make an API call to the reload_secure_credentials API
// to reload reloadable settings after the keystore has been updated.
func (c esClient) ReloadSecureSettings() error {
	esClient, err := c.newEsClientWithCACerts()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// TODO this is problematic as this call is supposed to happen only when all nodes have the updated
	// keystore which is something we cannot guarantee from this process. Also this call will be issued
	// on each node which is redundant and might be problematic as well.
	return esClient.ReloadSecureSettings(ctx)
}

// waitForEsReady waits for Elasticsearch to be ready while requesting the cluster info API
// and using the default client timeout.
func (c esClient) WaitForEsReady() {
	esClient, err := c.newEsClientWithCACerts()
	if err != nil {
		log.Error(err, "Cannot create Elasticsearch client with CA certs")
		return
	}

	for {
		ctx, cancel := context.WithTimeout(context.Background(), client.DefaultReqTimeout)
		_, err := esClient.GetClusterInfo(ctx)
		cancel()
		if err == nil {
			break
		}
		log.Info("Waiting for Elasticsearch to be ready")
		time.Sleep(waitEsReadinessPeriod)
	}
}

// newEsClientWithCACerts create a new Elasticsearch client configured with CA certs
func (c esClient) newEsClientWithCACerts() (client.Client, error) {
	caCerts, err := loadCerts(c.CACertsPath)
	if err != nil {
		return nil, err
	}

	return client.NewElasticsearchClient(nil, c.endpoint, c.user, c.version, caCerts), nil
}

// loadCerts returns the certificates given a certificates path.
func loadCerts(caCertPath string) ([]*x509.Certificate, error) {
	bytes, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	return certificates.ParsePEMCerts(bytes)
}
