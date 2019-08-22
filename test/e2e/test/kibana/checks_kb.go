// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	kbtype "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type kbChecks struct {
	client *http.Client
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	if b.Kibana.Spec.NodeCount == 0 {
		return test.StepList{}
	}

	checks := kbChecks{
		client: test.NewHTTPClient(nil),
	}
	return test.StepList{
		checks.CreateKbClient(b.Kibana),
		checks.CheckKbLoginHealthy(b.Kibana),
	}
}

func (check *kbChecks) CreateKbClient(kb kbtype.Kibana) test.Step {
	return test.Step{
		Name: "Create Kibana client",
		Test: func(t *testing.T) {
			k := test.NewK8sClientOrFatal()
			client, err := NewKibanaClient(kb, k)
			require.NoError(t, err)
			check.client = client
		},
	}
}

// CheckKbLoginHealthy checks that Kibana is able to connect to Elasticsearch by inspecting its login page.
func (check *kbChecks) CheckKbLoginHealthy(kb kbtype.Kibana) test.Step {
	return test.Step{
		Name: "Kibana should be able to connect to Elasticsearch",
		Test: test.Eventually(func() error {
			scheme := "http"
			if kb.Spec.HTTP.TLS.Enabled() {
				scheme = "https"
			}
			resp, err := check.client.Get(fmt.Sprintf("%s://%s.%s.svc:5601", scheme, name.HTTPService(kb.Name), kb.Namespace))

			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			// this is of course fragile and relying on potentially version specific implementation detail
			// verified to be present in 6.x and 7.x
			if !strings.Contains(string(body), "allowLogin&quot;:true") {
				return errors.New("initial Kibana UI state forbids login which indicates an error in the ES/Kibana setup")
			}
			return nil
		}),
	}
}
