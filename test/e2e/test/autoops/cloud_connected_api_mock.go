// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
)

// cloudConnectedAPIMockTemplate is the embedded YAML template for the Cloud Connected API mock.

//go:embed cloud_connected_api_mock.yaml
var cloudConnectedAPIMockTemplate string

// cloudConnectedAPIMockName returns the name for Cloud Connected API mock resources based on the test run name.
func cloudConnectedAPIMockName() string {
	return fmt.Sprintf("wiremock-%s", test.Ctx().TestRun)
}

// cloudConnectedAPIMockSelectorLabels returns the selector labels for Cloud Connected API mock resources.
func cloudConnectedAPIMockSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "wiremock-cloud-connected-api",
		"app.kubernetes.io/instance": cloudConnectedAPIMockName(),
	}
}

// deployCloudConnectedAPIMock deploys the mock service for Cloud Connected API using the YAML template.
func deployCloudConnectedAPIMock(k *test.K8sClient) error {
	ctx := context.Background()

	// Render the template
	objects, err := renderCloudConnectedAPIMockTemplate()
	if err != nil {
		return fmt.Errorf("failed to render Cloud Connected API mock template: %w", err)
	}

	if len(objects) == 0 {
		return fmt.Errorf("no objects rendered from template")
	}

	// Create or update each object
	for i, obj := range objects {
		if err := k.CreateOrUpdate(obj); err != nil {
			return fmt.Errorf("failed to create/update object %d (%T, %s/%s): %w", i, obj, obj.GetNamespace(), obj.GetName(), err)
		}
	}

	// Wait for deployment to be ready
	return waitForCloudConnectedAPIMockReady(ctx, k, test.Ctx().E2ENamespace)
}

// deleteCloudConnectedAPIMock deletes all Cloud Connected API mock resources.
func deleteCloudConnectedAPIMock(k *test.K8sClient) error {
	ctx := context.Background()
	namespace := test.Ctx().E2ENamespace
	name := cloudConnectedAPIMockName()

	// Delete Service
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if err := k.Client.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Delete Deployment
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if err := k.Client.Delete(ctx, deploy); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Delete ConfigMaps
	mappingsCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name + "-mappings", Namespace: namespace}}
	if err := k.Client.Delete(ctx, mappingsCM); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	filesCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name + "-files", Namespace: namespace}}
	if err := k.Client.Delete(ctx, filesCM); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// renderCloudConnectedAPIMockTemplate renders the embedded YAML template with test context values.
func renderCloudConnectedAPIMockTemplate() ([]k8sclient.Object, error) {
	// Parse and execute the embedded template
	tmpl, err := template.New("cloud-connected-api-mock").Funcs(sprig.TxtFuncMap()).Parse(cloudConnectedAPIMockTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, test.Ctx()); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// Decode the YAML into Kubernetes objects using helper.YAMLDecoder
	decoder := helper.NewYAMLDecoder()
	runtimeObjects, err := decoder.ToObjects(bufio.NewReader(&rendered))
	if err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	// Convert runtime.Object to client.Object
	objects := make([]k8sclient.Object, 0, len(runtimeObjects))
	for _, obj := range runtimeObjects {
		clientObj, ok := obj.(k8sclient.Object)
		if !ok {
			return nil, fmt.Errorf("object %T does not implement client.Object", obj)
		}
		objects = append(objects, clientObj)
	}

	return objects, nil
}

func waitForCloudConnectedAPIMockReady(ctx context.Context, k *test.K8sClient, namespace string) error {
	var pods corev1.PodList
	if err := k.Client.List(ctx, &pods,
		k8sclient.InNamespace(namespace),
		k8sclient.MatchingLabels(cloudConnectedAPIMockSelectorLabels()),
	); err != nil {
		return err
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no Cloud Connected API mock pods found")
	}

	for _, pod := range pods.Items {
		if !k8s.IsPodReady(pod) {
			return fmt.Errorf("Cloud Connected API mock pod %s not ready", pod.Name)
		}
	}

	return nil
}
