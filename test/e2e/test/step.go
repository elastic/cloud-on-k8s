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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
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
			// Only run eck diagnostics after each run when job is
			// run from CI, which provides a 'job-name' flag.
			if ctx.JobName != "" {
				logf.Log.Info("running eck-diagnostics job")
				if err := runECKDiagnostics(ctx, ts); err != nil {
					logf.Log.Error(err, "while running eck diagnostics")
				} else {
					uploadDiagnosticsArtifacts()
				}
			}
			if err := deleteElasticResources(); err != nil {
				logf.Log.Error(err, "while deleting elastic resources")
				return
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
	cmd := exec.Command("eck-diagnostics", "--output-directory", "/tmp", "-o", ctx.Operator.Namespace, "-r", strings.Join(otherNS, ","), "--run-agent-diagnostics")
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

func createECKDiagnosticsJob(name, version, operatorNamespace, e2eNamespace, svcAccount string, managedNamespaces []string) batchv1.Job {
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: e2eNamespace,
			Labels: map[string]string{
				"eck-diagnostics": name,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: "alpine",
							Command: []string{
								"/bin/sh", "-c",
							},
							Args: []string{
								strings.Join([]string{
									fmt.Sprintf("wget -O /tmp/diagnostics.tar.gz https://github.com/elastic/eck-diagnostics/releases/download/%[1]s/eck-diagnostics_%[1]s_Linux_x86_64.tar.gz", version),
									"cd /tmp",
									"tar -zxf diagnostics.tar.gz",
									fmt.Sprintf("HOME=/tmp ./eck-diagnostics --output-directory /tmp -o %s, -r %s --run-agent-diagnostics", operatorNamespace, strings.Join(append([]string{e2eNamespace}, managedNamespaces...), ",")),
								}, " && "),
							},
						},
					},
					ServiceAccountName: svcAccount,
					RestartPolicy:      corev1.RestartPolicyNever,
				},
			},
			BackoffLimit: pointer.Int32(1),
		},
	}
}

func startJob(job batchv1.Job) error {
	client, err := NewK8sClient()
	if err != nil {
		return fmt.Errorf("while creating k8s client: %w", err)
	}
	err = client.Client.Create(context.Background(), &job)
	if err != nil {
		return fmt.Errorf("while creating job: %w", err)
	}
	return nil
}

func waitForJob(ctx context.Context, client *kubernetes.Clientset, job batchv1.Job) error {
	watch, err := client.CoreV1().Pods(job.GetNamespace()).Watch(ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(job.Labels),
	})
	if err != nil {
		return fmt.Errorf("while retrieving watcher for job: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			logf.Log.Error(fmt.Errorf("job for %s did not complete", job.GetName()), "while waiting for job to complete")
			return nil
		case event := <-watch.ResultChan():
			logf.Log.Info("event type: %v", event.Type)
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			switch pod.Status.Phase {
			case corev1.PodRunning, corev1.PodPending:
				continue
			case corev1.PodFailed:
				logf.Log.Error(fmt.Errorf("while running eck-diagnostics for %s: %s", job.GetName(), pod.Status.String()), "pod failed")
				return nil
			case corev1.PodSucceeded:
				return nil
			}
		}
	}
}

func createK8SClient() (*kubernetes.Clientset, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func uploadDiagnosticsArtifacts() {
	ctx := Ctx()
	cmd := exec.Command("gsutil", "cp", "/tmp/*.zip", fmt.Sprintf("gs://eck-e2e-buildkite-artifacts/jobs/%s/%s/", ctx.JobName, ctx.BuildNumber)) //nolint:gosec
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
