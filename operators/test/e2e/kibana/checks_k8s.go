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

	kbname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/params"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// K8sStackChecks returns all test steps to verify the given stack
// in K8s is the expected one
func K8sStackChecks(stack Builder, k8sClient *helpers.K8sHelper) helpers.TestStepList {
	return helpers.TestStepList{
		CheckKibanaDeployment(stack, k8sClient),
		CheckKibanaPodsCount(stack, k8sClient),
		CheckKibanaPodsRunning(stack, k8sClient),
		CheckServices(stack, k8sClient),
		CheckServicesEndpoints(stack, k8sClient),
	}
}

// CheckKibanaDeployment checks that Kibana deployment exists
func CheckKibanaDeployment(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana deployment should be set",
		Test: helpers.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(types.NamespacedName{
				Namespace: params.Namespace,
				Name:      kbname.Deployment(stack.Kibana.Name),
			}, &dep)
			if stack.Kibana.Spec.NodeCount == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != stack.Kibana.Spec.NodeCount {
				return fmt.Errorf("invalid Kibana replicas count: expected %d, got %d", stack.Kibana.Spec.NodeCount, *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckKibanaPodsCount checks that Kibana pods count matches the expected one
func CheckKibanaPodsCount(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana pods count should match the expected one",
		Test: helpers.Eventually(func() error {
			return k.CheckPodCount(helpers.KibanaPodListOptions(stack.Kibana.Name), int(stack.Kibana.Spec.NodeCount))
		}),
	}
}

// CheckKibanaPodsRunning checks that all ES pods for the given stack are running
func CheckKibanaPodsRunning(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana pods should eventually be running",
		Test: helpers.Eventually(func() error {
			pods, err := k.GetPods(helpers.KibanaPodListOptions(stack.Kibana.Name))
			if err != nil {
				return err
			}
			for _, p := range pods {
				if p.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("pod not running yet")
				}
			}
			return nil
		}),
	}
}

// CheckServices checks that all stack services are created
func CheckServices(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana services should be created",
		Test: helpers.Eventually(func() error {
			for _, s := range []string{
				kbname.HTTPService(stack.Kibana.Name),
			} {
				if _, err := k.GetService(s); err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// CheckServicesEndpoints checks that services have the expected number of endpoints
func CheckServicesEndpoints(stack Builder, k *helpers.K8sHelper) helpers.TestStep {
	return helpers.TestStep{
		Name: "Kibana services should have endpoints",
		Test: helpers.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				kbname.HTTPService(stack.Kibana.Name): int(stack.Kibana.Spec.NodeCount),
			} {
				if addrCount == 0 {
					continue // maybe no Kibana in this stack
				}
				endpoints, err := k.GetEndpoints(endpointName)
				if err != nil {
					return err
				}
				if len(endpoints.Subsets) == 0 {
					return fmt.Errorf("no subset for endpoint %s", endpointName)
				}
				if len(endpoints.Subsets[0].Addresses) != addrCount {
					return fmt.Errorf("%d addresses found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses), endpointName, addrCount)
				}
			}
			return nil
		}),
	}
}

// DoKibanaReq executes an HTTP request against a Kibana instance.
func DoKibanaReq(k *helpers.K8sHelper, stack Builder, method string, uri string, body []byte) ([]byte, error) {
	password, err := k.GetElasticPassword(stack.Kibana.Spec.ElasticsearchRef.Name)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(fmt.Sprintf("http://%s.%s:5601", kbname.HTTPService(stack.Kibana.Name), stack.Kibana.Namespace))
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
	req.Header.Set("kbn-version", stack.Kibana.Spec.Version)
	client := helpers.NewHTTPClient()
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
