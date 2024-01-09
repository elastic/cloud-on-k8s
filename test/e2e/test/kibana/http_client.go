// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/pkg/errors"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	commonhttp "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/http"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/network"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func NewKibanaClient(kb kbv1.Kibana, k *test.K8sClient) (*http.Client, error) {
	var caCerts []*x509.Certificate
	if kb.Spec.HTTP.TLS.Enabled() {
		crts, err := k.GetHTTPCerts(kbv1.KBNamer, kb.Namespace, kb.Name)
		if err != nil {
			return nil, err
		}
		caCerts = crts
	}
	return test.NewHTTPClient(caCerts), nil
}

// DoRequest executes an HTTP request against a Kibana instance using the given password for the elastic user.
func DoRequest(k *test.K8sClient, kb kbv1.Kibana, password string, method string, pathAndQuery string, body []byte, extraHeaders http.Header) ([]byte, http.Header, error) {
	scheme := "http"
	if kb.Spec.HTTP.TLS.Enabled() {
		scheme = "https"
	}
	// add .svc suffix so that requests work when using the port-forwarder during local test runs
	u, err := url.Parse(fmt.Sprintf("%s://%s.%s.svc:%d", scheme, kbv1.HTTPService(kb.Name), kb.Namespace, network.HTTPPort))
	if err != nil {
		return nil, http.Header{}, errors.Wrap(err, "while parsing url")
	}

	pathAndQueryURL, err := url.Parse(pathAndQuery)
	if err != nil {
		return nil, http.Header{}, errors.Wrap(err, "while parsing path and query from caller")
	}

	u.Path = pathAndQueryURL.Path
	u.RawQuery = pathAndQueryURL.RawQuery

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(body)) //nolint:noctx
	if err != nil {
		return nil, http.Header{}, errors.Wrap(err, "while creating request")
	}

	req.SetBasicAuth("elastic", password)
	req.Header.Set("Content-Type", "application/json")
	// send the kbn-version header expected by the Kibana server to protect against xsrf attacks
	req.Header.Set("kbn-version", kb.Spec.Version)

	// add any extra headers
	for name, values := range extraHeaders {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	client, err := NewKibanaClient(kb, k)
	if err != nil {
		return nil, http.Header{}, errors.Wrap(err, "while creating kibana client")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, http.Header{}, errors.Wrap(err, "while doing request")
	}
	defer resp.Body.Close()
	if err := commonhttp.MaybeAPIError(resp); err != nil {
		return nil, http.Header{}, err
	}

	var respBody []byte
	if respBody, err = io.ReadAll(resp.Body); err != nil {
		return nil, http.Header{}, err
	}

	return respBody, resp.Header, nil
}
