// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"fmt"
	"net"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

type pmClientFactory func(restartContext RestartContext, pod corev1.Pod) (processmanager.Client, error)

// createProcessManagerClient creates a client to interact with the pod's process manager.
func createProcessManagerClient(restartContext RestartContext, pod corev1.Pod) (processmanager.Client, error) {
	podIP := net.ParseIP(pod.Status.PodIP)
	url := fmt.Sprintf("https://%s:%d", podIP.String(), processmanager.DefaultPort)
	rawCA, err := nodecerts.GetCA(restartContext.K8sClient, k8s.ExtractNamespacedName(&restartContext.Cluster.ObjectMeta))
	if err != nil {
		return nil, err
	}
	certs, err := certificates.ParsePEMCerts(rawCA)
	if err != nil {
		return nil, err
	}
	return processmanager.NewClient(url, certs, restartContext.Dialer), nil
}
