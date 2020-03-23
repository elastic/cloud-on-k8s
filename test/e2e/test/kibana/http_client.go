// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	kbname "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/pkg/errors"
)

func NewKibanaClient(kb kbv1.Kibana, k *test.K8sClient) (*http.Client, error) {
	var caCerts []*x509.Certificate
	if kb.Spec.HTTP.TLS.Enabled() {
		crts, err := k.GetHTTPCerts(kbname.KBNamer, kb.Namespace, kb.Name)
		if err != nil {
			return nil, err
		}
		caCerts = crts
	}
	return test.NewHTTPClient(caCerts), nil
}

// DoRequest executes an HTTP request against a Kibana instance using the given password for the elastic user.
func DoRequest(k *test.K8sClient, kb kbv1.Kibana, password string, method string, uri string, body []byte) ([]byte, error) {
	scheme := "http"
	if kb.Spec.HTTP.TLS.Enabled() {
		scheme = "https"
	}
	// add .svc suffix so that requests work when using the port-forwarder during local test runs
	u, err := url.Parse(fmt.Sprintf("%s://%s.%s.svc:5601", scheme, kbname.HTTPService(kb.Name), kb.Namespace))
	if err != nil {
		return nil, errors.Wrap(err, "while parsing url")
	}

	u.Path = path.Join(u.Path, uri)
	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrap(err, "while creating request")
	}

	req.SetBasicAuth("elastic", password)
	req.Header.Set("Content-Type", "application/json")
	// send the kbn-version header expected by the Kibana server to protect against xsrf attacks
	req.Header.Set("kbn-version", kb.Spec.Version)
	client, err := NewKibanaClient(kb, k)
	if err != nil {
		return nil, errors.Wrap(err, "while creating kibana client")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "while doing request")
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fail to request %s, status is %d)", uri, resp.StatusCode)
	}

	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}
