// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"crypto/x509"
	"net/http"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
)

func NewKibanaClient(kb v1alpha1.Kibana, k *K8sHelper) (*http.Client, error) {
	var caCerts []*x509.Certificate
	if kb.Spec.HTTP.TLS.Enabled() {
		crts, err := k.GetHTTPCerts(name.KBNamer, kb.Name)
		if err != nil {
			return nil, err
		}
		caCerts = crts
	}
	return NewHTTPClient(caCerts), nil
}
