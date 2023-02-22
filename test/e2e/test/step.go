// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Step represents a single test
type Step struct {
	Name      string
	Test      func(t *testing.T)
	Skip      func() bool // returns true if the test should be skipped
	OnFailure func()      // additional step specific callback on failure
}

// StepList defines a list of Step
type StepList []Step

// WithSteps appends the given StepList to the StepList
func (l StepList) WithSteps(testSteps StepList) StepList {
	return append(l, testSteps...)
}

// WithStep appends the given Step to the StepList
func (l StepList) WithStep(testStep Step) StepList {
	return append(l, testStep)
}

// RunSequential runs the StepList sequentially, continues on any errors.
// Runs eck-diagnostics and uploads artifacts when run within the CI system.
//
//nolint:thelper
func (l StepList) RunSequential(t *testing.T) {
	once := sync.Once{}
	for _, ts := range l {
		if ts.Skip != nil && ts.Skip() {
			log.Info("Skipping test", "name", ts.Name)
			continue
		}
		if !t.Run(ts.Name, ts.Test) {
			logf.Log.Error(errors.New("test failure"), "continuing with additional tests")
			if ts.OnFailure != nil {
				ts.OnFailure()
			}
			ctx := Ctx()
			defer func() {
				err := deleteElasticResources()
				if err != nil {
					logf.Log.Error(err, "while deleting elastic resources")
				}
			}()
			// Only run eck diagnostics after each run when job is
			// run from CI, which provides a 'job-name' flag.
			if ctx.JobName != "" {
				once.Do(initGSUtil)
				logf.Log.Info("running eck-diagnostics job")
				runECKDiagnostics(ctx, t.Name(), ts)
				uploadDiagnosticsArtifacts()
			}
		}
	}
}

func initGSUtil() {
	cmd := exec.Command("gcloud", "auth", "activate-service-account", "--key-file=/etc/gcp/credentials.json")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("Failed to initialize gsutil: %s", err))
	}
}

func runECKDiagnostics(ctx Context, testName string, step Step) {
	otherNS := append([]string{ctx.E2ENamespace}, ctx.Operator.ManagedNamespaces...)
	fullTestName := fmt.Sprintf("%s-%s", testName, step.Name)
	normalizedTestName := strings.ReplaceAll(strings.ReplaceAll(fullTestName, " ", "_"), "/", "-")
	cmd := exec.Command("eck-diagnostics", "--output-directory", "/tmp", "-n", fmt.Sprintf("eck-diagnostic-%s.zip", normalizedTestName), "-o", ctx.Operator.Namespace, "-r", strings.Join(otherNS, ","), "--run-agent-diagnostics") //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("Failed to run eck-diagnostics: %s", err))
	}
}

func uploadDiagnosticsArtifacts() {
	ctx := Ctx()
	cmd := exec.Command("gsutil", "cp", "/tmp/*.zip", fmt.Sprintf("gs://eck-e2e-buildkite-artifacts/jobs/%s/%s/", ctx.JobName, ctx.BuildNumber)) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("Failed to run gsutil: %s", err))
	}
}

func ensureTmpHomeEnv(env []string) []string {
	found := false
	for i, e := range env {
		if strings.Contains(e, "HOME=") {
			env[i] = "HOME=/tmp"
			found = true
		}
	}
	if !found {
		env = append(env, "HOME=/tmp")
	}
	return env
}

func deleteElasticResources() error {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "while getting in cluster config")
		return err
	}
	clntset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "while getting clientset")
		return err
	}

	type version string
	groupVersionMap := map[string][]version{}

	apiGroups, _, _ := clntset.Discovery().ServerGroupsAndResources()

	for _, group := range apiGroups {
		if !strings.Contains(strings.ToLower(group.Name), "elastic") {
			continue
		}
		for _, ver := range group.Versions {
			groupVersionMap[group.Name] = append(groupVersionMap[group.Name], version(ver.Version))
		}
	}

	expander := restmapper.NewDiscoveryCategoryExpander(clntset)
	groupResources, ok := expander.Expand("elastic")
	if !ok {
		err = fmt.Errorf("while running 'kubectl delete elastic'")
		log.Error(err, "while finding elastic categories in cluster")
		return err
	}
	namespace := Ctx().E2ENamespace
	dynamicClient := dynamic.New(clntset.RESTClient())
	for _, gr := range groupResources {
		for _, v := range groupVersionMap[gr.Group] {
			if err := dynamicClient.Resource(schema.GroupVersionResource{
				Group:    gr.Group,
				Resource: gr.Resource,
				Version:  string(v),
			}).Namespace(namespace).DeleteCollection(context.Background(), v1.DeleteOptions{}, v1.ListOptions{}); err != nil && !api_errors.IsNotFound(err) {
				msg := fmt.Sprintf("while deleting elastic resources in %s", namespace)
				log.Error(err, msg, "group", gr.Group, "resource", gr.Resource)
				return err
			}
		}
	}
	return nil
}

type StepsFunc func(k *K8sClient) StepList

func EmptySteps(_ *K8sClient) StepList {
	return StepList{}
}
