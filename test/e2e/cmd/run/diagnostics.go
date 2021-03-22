// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package run

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	v1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

type diagnosticContext struct {
	test.Context
	ESNamespace string
	ESName      string
	PodName     string
	TLS         bool
}

func (h *helper) runEsDiagnosticsJob() {
	output, err := h.kubectl("get", "es", "--all-namespaces", "-o", "json")
	if err != nil {
		log.Error(err, "failed to list Elasticsearch clusters")
		return
	}
	var ess v1.ElasticsearchList
	if err := json.Unmarshal([]byte(output), &ess); err != nil {
		log.Error(err, "failed to unmarshal kubectl response")
	}

	if len(ess.Items) == 0 {
		log.Info("No Elasticsearch clusters deployed. Cannot extract diagnostics")
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(ess.Items))

	diagnosticsTimeout := 10 * time.Minute

	for _, es := range ess.Items {
		go func(es v1.Elasticsearch) {
			defer wg.Done()
			podName := fmt.Sprintf("diag-%s", es.Name)
			err := h.kubectlApplyTemplateWithCleanup("config/e2e/diagnostics_job.yaml", diagnosticContext{
				Context:     h.testContext,
				ESNamespace: es.Namespace,
				ESName:      es.Name,
				PodName:     podName,
				TLS:         es.Spec.HTTP.TLS.Enabled(),
			})
			if err != nil {
				log.Error(err, "diagnostics pod failed to create")
				return
			}

			wait := exec.Command("kubectl", "wait", //nolint:gosec
				fmt.Sprintf("--timeout=%s", diagnosticsTimeout.String()),
				"--for=condition=ContainersReady",
				"-n", es.Namespace,
				fmt.Sprintf("pod/%s", podName),
			)
			out, err := wait.CombinedOutput()
			if err != nil {
				log.Error(err, "diagnostics pod did not complete successfully", "pod", podName, "output", string(out))
				return
			}

			// copy the whole diagnostic-output directory as archive names are unpredictable into a temporary folder named after the cluster
			// assumption: cluster names in e2e tests are unique
			cp := exec.Command("kubectl", "cp", fmt.Sprintf("%s/%s:/diagnostic-output", es.Namespace, podName), es.Name) //nolint:gosec
			out, err = cp.CombinedOutput()
			if err != nil {
				log.Error(err, "diagnostics output did not copy successfully", "pod", podName, "output", string(out))
				return
			}
			log.Info("Copied diagnostics", "name", podName, "namespace", es.Namespace)

			err = h.normalizeDiagnosticArchives(es.Name)
			if err != nil {
				log.Error(err, "error while normalizing diagnostic archives")
			}

			// clean up the download directory
			out, err = exec.Command("rm", "-r", es.Name).CombinedOutput() //nolint:gosec
			if err != nil {
				log.Error(err, "while deleting download directory", "output", out)
			}
		}(es)
	}
	wg.Wait()
}

func forEachFile(pattern string, fn func(string) ([]byte, error)) error {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, file := range files {
		out, err := fn(file)
		if err != nil {
			log.Info(string(out))
			return err
		}
	}
	return nil
}

// normalizeDiagnosticArchives  normalizes everything to *.tgz. This is because CI piplines are configured to upload *.tgz,
// support-diagnostics produces either *.zip or if that fails *.tar.gz (!). It also incorporate the dirName parameter in
// the name of the archive to avoid overwriting archives from multiple clusters with the same timestamp.
func (h *helper) normalizeDiagnosticArchives(dirName string) error {
	err := forEachFile(fmt.Sprintf("%s/api-diagnostics*.zip", dirName), func(file string) ([]byte, error) {
		return exec.Command("tar", "czf", fmt.Sprintf("api-diagnostics-%s.tgz", dirName), fmt.Sprintf("@%s", file)).CombinedOutput() //nolint:gosec
	})
	if err != nil {
		return err
	}

	return forEachFile(fmt.Sprintf("%s/api-diagnostics*.tar.gz", dirName), func(file string) ([]byte, error) {
		return exec.Command("mv", file, fmt.Sprintf("api-diagnostics-%s.tgz", dirName)).CombinedOutput()
	})
}
