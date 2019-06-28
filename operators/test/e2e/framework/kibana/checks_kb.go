// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/framework"
	"github.com/pkg/errors"
)

type kbChecks struct {
	client http.Client
}

func (b Builder) CheckStackTestSteps(k *framework.K8sClient) framework.TestStepList {
	if b.Kibana.Spec.NodeCount == 0 {
		return framework.TestStepList{}
	}
	checks := kbChecks{
		client: framework.NewHTTPClient(),
	}
	return framework.TestStepList{
		checks.CheckKbLoginHealthy(b.Kibana),
	}
}

// CheckKbLoginHealthy checks that Kibana is able to connect to Elasticsearch by inspecting its login page.
func (check *kbChecks) CheckKbLoginHealthy(kb kbtype.Kibana) framework.TestStep {
	return framework.TestStep{
		Name: "Kibana should be able to connect to Elasticsearch",
		Test: framework.Eventually(func() error {
			resp, err := check.client.Get(fmt.Sprintf("http://%s.%s.svc:5601", name.HTTPService(kb.Name), kb.Namespace))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			// this is of course fragile and relying on potentially version specific implementation detail
			// verified to be present in 6.x and 7.x
			if !strings.Contains(string(body), "allowLogin&quot;:true") {
				return errors.New("initial Kibana UI state forbids login which indicates an error in the ES/Kibana setup")
			}
			return nil
		}),
	}
}
