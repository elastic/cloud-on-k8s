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

	api_errors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	gcpCredentialsFile = "/var/run/secrets/e2e/gcp-credentials.json" //nolint:gosec
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
	if _, err := os.Stat(gcpCredentialsFile); err != nil && errors.Is(err, fs.ErrNotExist) {
		return false
	} else if err != nil {
		log.Error(err, "while checking for existence of %s", gcpCredentialsFile)
		return false
	}
	if ctx.JobName == "" {
		return false
	}
	return true
}

func initGSUtil() {
	cmd := exec.Command("gcloud", "auth", "activate-service-account", "--key-file=/etc/gcp/credentials.json")
	setupStdOutErr(cmd)
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		log.Error(err, fmt.Sprintf("while initializing gsutil: %s", err))
	}
	// if _, _, err := command.New("gsutil", []string{
	// 	"auth", "activate-service-account", "--key-file=/var/run/secrets/e2e/gcp-credentials.json",
	// }...).WithEnv("HOME=/tmp").Build().Execute(context.Background()); err != nil {
	// 	log.Error(err, fmt.Sprintf("while initializing gsutil: %s", err))
	// }
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
	// cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		// if _, _, err := command.New("eck-diagnostics", []string{
		// 	"--output-directory", "/tmp",
		// 	"-n", fmt.Sprintf("eck-diagnostic-%s.zip", normalizedTestName),
		// 	"-o", ctx.Operator.Namespace,
		// 	"-r", strings.Join(otherNS, ","),
		// 	"--run-agent-diagnostics",
		// }...).Build().Execute(context.Background()); err != nil {
		// 	log.Error(err, fmt.Sprintf("while running eck-diagnostics: %s", err))
		// }
		// temporarily disabling to verify why this is needed for eck-diagnostics.
		// cmd.Env = ensureTmpHomeEnv(cmd.Environ())
		log.Error(err, fmt.Sprintf("while running eck-diagnostics: %s", err))
	}
}

func uploadDiagnosticsArtifacts() {
	ctx := Ctx()
	cmd := exec.Command("gsutil", "cp", "/tmp/*.zip", fmt.Sprintf("gs://eck-e2e-buildkite-artifacts/jobs/%s/%s/", ctx.JobName, ctx.BuildNumber)) //nolint:gosec
	setupStdOutErr(cmd)
	cmd.Env = ensureTmpHomeEnv(cmd.Environ())
	if err := cmd.Run(); err != nil {
		// if _, _, err := command.New("gsutil", []string{
		// 	"cp", "/tmp/*.zip", fmt.Sprintf("gs://eck-e2e-buildkite-artifacts/jobs/%s/%s/", ctx.JobName, ctx.BuildNumber),
		// }...).WithEnv("HOME=/tmp").Build().Execute(context.Background()); err != nil {
		log.Error(err, fmt.Sprintf("while running gsutil: %s", err))
	}
}

func setupStdOutErr(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

// GSUtil command requires a valid writable home directory to be set to function properly,
// as it writes some of it's configuration locally when running, so /tmp is used.
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

	type version string
	groupVersionMap := map[string][]version{}

	apiGroups, _, err := clntset.Discovery().ServerGroupsAndResources()
	if err != nil {
		log.Error(err, "while running kubernetes client discovery")
		return err
	}

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
