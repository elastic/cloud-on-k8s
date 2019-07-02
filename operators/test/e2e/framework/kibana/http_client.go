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

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/pkg/errors"
)

func NewKibanaClient(kb v1alpha1.Kibana, k *framework.K8sClient) (*http.Client, error) {
	var caCerts []*x509.Certificate
	if kb.Spec.HTTP.TLS.Enabled() {
		crts, err := k.GetHTTPCerts(name.KBNamer, kb.Name)
		if err != nil {
			return nil, err
		}
		caCerts = crts
	}
	return framework.NewHTTPClient(caCerts), nil
}

// DoKibanaReq executes an HTTP request against a Kibana instance.
func DoKibanaReq(k *framework.K8sClient, b Builder, method string, uri string, body []byte) ([]byte, error) {
	password, err := k.GetElasticPassword(b.Kibana.Spec.ElasticsearchRef.Name)
	if err != nil {
		return nil, errors.Wrap(err, "while getting elastic password")
	}
	scheme := "http"
	if b.Kibana.Spec.HTTP.TLS.Enabled() {
		scheme = "https"
	}
	u, err := url.Parse(fmt.Sprintf("%s://%s.%s:5601", scheme, kbname.HTTPService(b.Kibana.Name), b.Kibana.Namespace))
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
	req.Header.Set("kbn-version", b.Kibana.Spec.Version)
	client, err := NewKibanaClient(b.Kibana, k)
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
