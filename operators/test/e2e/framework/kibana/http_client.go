// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
)

// DoKibanaReq executes an HTTP request against a Kibana instance.
func DoKibanaReq(b Builder, k *framework.K8sClient, method string, uri string, body []byte) ([]byte, error) {
	password, err := k.GetElasticPassword(b.Kibana.Spec.ElasticsearchRef.Name)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(fmt.Sprintf("http://%s.%s:5601", name.HTTPService(b.Kibana.Name), b.Kibana.Namespace))
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(u.Path, uri)
	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth("elastic", password)
	req.Header.Set("Content-Type", "application/json")
	// send the kbn-version header expected by the Kibana server to protect against xsrf attacks
	req.Header.Set("kbn-version", b.Kibana.Spec.Version)
	client := framework.NewHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fail to request %s, status is %d)", uri, resp.StatusCode)
	}

	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}
