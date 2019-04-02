// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/pkg/errors"
)

type kbChecks struct {
	client http.Client
}

// Kibana checks returns all test steps to verify the given stack's Kibana
// deployment is running as expected.
func KibanaChecks(kb kbtype.Kibana, licenseType estype.LicenseType) helpers.TestStepList {
	if kb.Spec.NodeCount == 0 {
		return helpers.TestStepList{}
	}
	checks := kbChecks{
		client: helpers.NewHTTPClient(),
	}
	if licenseType == estype.LicenseTypeBasic {
		return helpers.TestStepList{
			checks.CheckKbStatusHealthy(kb),
		}
	}
	return helpers.TestStepList{
		checks.CheckKbLoginHealthy(kb),
	}
}

// CheckKbLoginHealthy checks that Kibana is able to connect to Elasticsearch by inspecting its login page.
func (check *kbChecks) CheckKbLoginHealthy(kb kbtype.Kibana) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana should be able to connect to Elasticsearch",
		Test: helpers.Eventually(func() error {
			resp, err := check.client.Get(fmt.Sprintf("http://%s-kibana.%s.svc.cluster.local:5601", kb.Name, kb.Namespace))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			// this is of course fragile and relying on potentially version specific implementation detail
			// verified to be present in 6.x and 7.x
			if !strings.Contains(string(body), "allowLogin&quot;:true") {
				return errors.New("Initial Kibana UI state forbids login which indicates an error in the ES/Kibana setup")
			}
			return nil
		}),
	}
}

// CheckKbStatusHealthy checks that Kibana is able to connect to Elasticsearch when x-pack security is off.
func (check *kbChecks) CheckKbStatusHealthy(kb kbtype.Kibana) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana should be able to connect to Elasticsearch",
		Test: helpers.Eventually(func() error {
			// this will return a 503 on connectivity problems in 6.x and 7.x
			_, err := check.client.Get(fmt.Sprintf("http://%s-kibana.%s.svc.cluster.local:5601/app/kibana", kb.Name, kb.Namespace))
			return err
		}),
	}
}
