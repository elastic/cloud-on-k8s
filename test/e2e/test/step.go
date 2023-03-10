// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	// local variable for storing ctx.GCPCredentials path such
	// that initGSUtil can use it.
	gcpCredentialsPath string
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

// RunSequential runs the StepList sequentially, continuing on any errors.
// If the tests are running within CI, the following occurs:
//   - ECK-diagnostics is run after each failure
//   - The resulting Zip file is uploaded to a GS Bucket
//   - All Zip files are downloaded to local agent when tests complete and are
//     added as Buildkite artifacts.
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
				if err := deleteElasticResources(); err != nil {
					logf.Log.Error(err, "while deleting elastic resources")
				}
			}()
			if canRunDiagnostics(ctx) {
				gcpCredentialsPath = ctx.GCPCredentialsPath
				once.Do(initGSUtil)
				logf.Log.Info("running eck-diagnostics job")
				runECKDiagnostics(ctx, t.Name(), ts)
				uploadDiagnosticsArtifacts()
			}
		}
	}
}

// canRunDiagnostics will determine if this e2e test run has the ability to run eck-diagnostics after
// each test failure, which includes uploading the resulting zip file to a GS bucket. If the job name
// is set (not empty) and the google credentials file exists, then we should be able to run diagnostics.
func canRunDiagnostics(ctx Context) bool {
	if _, err := os.Stat(ctx.GCPCredentialsPath); err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	} else if err != nil {
		log.Error(err, "while checking for existence of %s", ctx.GCPCredentialsPath)
		return false
	}
	if ctx.JobName == "" {
		return false
	}
	return true
}

func initGSUtil() {
	cmd := exec.Command("gcloud", "auth", "activate-service-account", fmt.Sprintf("--key-file=%s", gcpCredentialsPath)) //nolint:gosec
	setupStdOutErr(cmd)
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("while initializing gsutil: %s", err))
	}
}

func runECKDiagnostics(ctx Context, testName string, step Step) {
	otherNS := append([]string{ctx.E2ENamespace}, ctx.Operator.ManagedNamespaces...)
	// The following appends the test, and it's sub-test names together with a '-'.
	// Example: For TestAutoscalingLegacy/Secrets_should_eventually_be_created:
	// testName: TestAutoscalingLegacy, step.Name: Secrets_should_eventually_be_created
	fullTestName := fmt.Sprintf("%s-%s", testName, step.Name)
	// Convert any spaces to "_", and "/" to "-" in the test name.
	normalizedTestName := strings.ReplaceAll(strings.ReplaceAll(fullTestName, " ", "_"), "/", "-")
	cmd := exec.Command("eck-diagnostics", "--output-directory", "/tmp", "-n", fmt.Sprintf("eck-diagnostic-%s.zip", normalizedTestName), "-o", ctx.Operator.Namespace, "-r", strings.Join(otherNS, ","), "--run-agent-diagnostics") //nolint:gosec
	setupStdOutErr(cmd)
	// If /tmp is not set as HOME, the k8s client can't cache responses,
	// and a single run of diagnostics is incredibly slow.
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("while running eck-diagnostics: %s", err))
	}
}

func uploadDiagnosticsArtifacts() {
	ctx := Ctx()
	cmd := exec.Command("gsutil", "cp", "/tmp/*.zip", fmt.Sprintf("gs://eck-e2e-buildkite-artifacts/jobs/%s/%s/", ctx.JobName, ctx.BuildNumber)) //nolint:gosec
	setupStdOutErr(cmd)
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("while running gsutil: %s", err))
	}
}

func setupStdOutErr(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

// GSUtil command requires a valid writable home directory to be set to function properly,
// as it writes some of its configuration locally when running, so /tmp is used.
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

// This simulates "kubectl delete elastic" in the e2e namespace.
func deleteElasticResources() error {
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "while getting kubernetes config")
		return err
	}
	clntset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "while getting clientset")
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	groupVersionToResourceListMap := map[string][]v1.APIResource{}

	_, resources, err := clntset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Error(err, "while running kubernetes client discovery")
		return err
	}

	for _, resource := range resources {
		if strings.Contains(resource.GroupVersion, "k8s.elastic.co") {
			groupVersionToResourceListMap[resource.GroupVersion] = resource.APIResources
		}
	}

	namespace := Ctx().E2ENamespace
	dynamicClient := dynamic.New(clntset.RESTClient())
	for gv, resources := range groupVersionToResourceListMap {
		gvSlice := strings.Split(gv, "/")
		if len(gvSlice) != 2 {
			continue
		}
		group, version := gvSlice[0], gvSlice[1]
		for _, resource := range resources {
			if err := dynamicClient.Resource(schema.GroupVersionResource{
				Group:    group,
				Resource: resource.Name,
				Version:  version,
			}).Namespace(namespace).DeleteCollection(ctx, v1.DeleteOptions{}, v1.ListOptions{}); err != nil && !api_errors.IsNotFound(err) {
				msg := fmt.Sprintf("while deleting elastic resources in %s", namespace)
				log.Error(err, msg, "group", group, "resource", resource.Name, "version", version)
				return err
			}
		}
	}

	list, err := clntset.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("while listing all secrets in namespace %s: %w", namespace, err)
	}
	for _, secret := range list.Items {
		if err := clntset.CoreV1().Secrets(namespace).Delete(ctx, secret.GetName(), v1.DeleteOptions{}); err != nil {
			return fmt.Errorf("while deleting secret %s in namespace %s: %w", secret.GetName(), namespace, err)
		}
	}
	return nil
}

type StepsFunc func(k *K8sClient) StepList

func EmptySteps(_ *K8sClient) StepList {
	return StepList{}
}
