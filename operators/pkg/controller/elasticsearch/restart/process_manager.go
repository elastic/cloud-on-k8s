// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

type pmClientFactory func(restartContext RestartContext, pod corev1.Pod) (processmanager.Client, error)

// createProcessManagerClient creates a client to interact with the pod's process manager.
func createProcessManagerClient(restartContext RestartContext, pod corev1.Pod) (processmanager.Client, error) {
	podIP := net.ParseIP(pod.Status.PodIP)
	url := fmt.Sprintf("https://%s:%d", podIP.String(), processmanager.DefaultPort)

	var publicCertsSecret corev1.Secret
	if err := restartContext.K8sClient.Get(
		http.PublicCertsSecretRef(k8s.ExtractNamespacedName(&restartContext.Cluster)),
		&publicCertsSecret,
	); err != nil {
		return nil, err
	}

	certs := make([]*x509.Certificate, 0)

	if certsData, ok := publicCertsSecret.Data[certificates.CertFileName]; ok {
		publicCerts, err := certificates.ParsePEMCerts(certsData)
		if err != nil {
			return nil, err
		}
		certs = append(certs, publicCerts...)
	}

	return processmanager.NewClient(url, certs, restartContext.Dialer), nil
}

// ensureESProcessStopped interacts with the process manager to stop the ES process.
func ensureESProcessStopped(pmClient processmanager.Client, podName string) (bool, error) {
	// request ES process stop (idempotent)
	ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
	defer cancel()
	log.V(1).Info("Requesting ES process stop", "pod_name", podName)
	status, err := pmClient.Stop(ctx)
	if err != nil {
		return false, err
	}

	// we got the current status back, check if the process is stopped
	if status.State != processmanager.Stopped {
		log.V(1).Info("ES process is not stopped yet", "pod_name", podName, "state", status.State)
		// not stopped yet, requeue
		return false, nil
	}

	log.V(1).Info("ES process successfully stopped", "pod_name", podName)
	return true, nil
}

// ensureESProcessStarted interacts with the process manager to ensure all ES processes are started.
func ensureESProcessStarted(pmClient processmanager.Client, podName string) (bool, error) {
	// request ES process start (idempotent)
	ctx, cancel := context.WithTimeout(context.Background(), processmanager.DefaultReqTimeout)
	defer cancel()
	log.V(1).Info("Requesting ES process start", "pod_name", podName)
	status, err := pmClient.Start(ctx)
	if err != nil {
		return false, err
	}

	// we got the current status back, check if the process is started
	if status.State != processmanager.Started {
		log.V(1).Info("ES process is not started yet", "pod_name", podName, "state", status.State)
		// not started yet, requeue
		return false, nil
	}

	log.V(1).Info("ES process successfully started", "pod_name", podName)
	return true, nil
}
