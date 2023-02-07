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
				logf.Log.Info("running eck-diagnostics job")
				err := runECKDiagnostics(ctx, ts)
				if err != nil {
					logf.Log.Error(err, "while running eck diagnostics")
					continue
				}
				uploadDiagnosticsArtifacts()
			}

		}
	}
}

func runECKDiagnostics(ctx Context, step Step) error {
	// job := createECKDiagnosticsJob(
	// 	fmt.Sprintf(step.Name),
	// 	"1.3.0",
	// 	ctx.Operator.Namespace,
	// 	ctx.E2ENamespace,
	// 	ctx.E2EServiceAccount,
	// 	ctx.Operator.ManagedNamespaces,
	// )
	// err := startJob(job)
	// if err != nil {
	// 	return fmt.Errorf("while starting eck diagnostics job: %w", err)
	// }
	// client, err := createK8SClient()
	// if err != nil {
	// 	return fmt.Errorf("while creating k8s client: %w", err)
	// }
	// waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	// defer cancel()
	// err = waitForJob(waitCtx, client, job)
	// if err != nil {
	// 	return fmt.Errorf("while waiting for eck diagnostics job to finish: %w", err)
	// }
	otherNS := append([]string{ctx.E2ENamespace}, ctx.Operator.ManagedNamespaces...)
	cmd := exec.Command("eck-diagnostics", "--output-directory", "/tmp", "-o", ctx.Operator.Namespace, "-r", strings.Join(otherNS, ","), "--run-agent-diagnostics") //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := cmd.Environ()
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
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("Failed to run eck-diagnostics: %s", err))
	}
	return nil
}

func uploadDiagnosticsArtifacts() {
	ctx := Ctx()
	cmd := exec.Command("gsutil", "cp", "/tmp/*.zip", fmt.Sprintf("gs://eck-e2e-buildkite-artifacts/jobs/%s/%s/", ctx.JobName, ctx.BuildNumber)) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := cmd.Environ()
	found := false
	for i, e := range env {
		if strings.Contains(e, "GOOGLE_APPLICATION_CREDENTIALS") {
			log.Info("found GOOGLE_APPLICATION_CREDENTIALS in env for gsutil command: %s", "key", e)
			googleEnv := strings.Split(e, "=")
			fileName := googleEnv[1]
			b, err := os.ReadFile(fileName)
			if err != nil {
				log.Error(err, "while reading google credentials file", "filename", fileName)
				return
			}
			log.Info("google application credentials file data", "data", string(b))
		}
		if strings.Contains(e, "HOME=") {
			env[i] = "HOME=/tmp"
			found = true
		}
	}
	if !found {
		env = append(env, "HOME=/tmp")
	}
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("Failed to run gsutil: %s", err))
	}
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
				err = fmt.Errorf("while deleting elastic resources in %s: %w ", namespace, err)
				log.Error(err, "group", gr.Group, "resource", gr.Resource)
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
