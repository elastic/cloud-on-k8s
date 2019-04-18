// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/annotation"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/network"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Quorum computes the quorum of a cluster given the number of masters.
func Quorum(nMasters int) int {
	if nMasters == 0 {
		return 0
	}
	return (nMasters / 2) + 1
}

// UpdateSeedHostsConfigMap updates the config map that contains the seed hosts. It returns true if a reconcile
// iteration should be triggered later because some pods don't have an IP yet.
func UpdateSeedHostsConfigMap(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	pods []corev1.Pod,
) (bool, error) {
	// Get the masters from the pods
	var masters []corev1.Pod
	for _, p := range pods {
		if label.IsMasterNode(p) {
			masters = append(masters, p)
		}
	}

	// Create an array with the pod IP of the current master nodes
	var seedHosts []string
	for _, master := range masters {
		if len(master.Status.PodIP) > 0 { // do not add pod with no IPs
			seedHosts = append(
				seedHosts,
				fmt.Sprintf("%s:%d", master.Status.PodIP, network.TransportPort),
			)
		}
	}

	// Check if we have to requeue because some pods don't have an IP yet
	requeue := len(seedHosts) != len(masters)

	var hosts string
	if seedHosts != nil {
		hosts = strings.Join(seedHosts, "\n")
	}
	expected := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.UnicastHostsConfigMap(es.Name),
			Namespace: es.Namespace,
			Labels:    label.NewLabels(k8s.ExtractNamespacedName(&es)),
		},
		Data: map[string]string{
			volume.UnicastHostsFile: hosts,
		},
	}

	reconciled := &corev1.ConfigMap{}
	return requeue, reconciler.ReconcileResource(
		reconciler.Params{
			Client:     c,
			Scheme:     scheme,
			Owner:      &es,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Data = expected.Data
			},
			PreCreate: func() {
				log.Info("Creating seed hosts", "hosts", seedHosts)
			},
			PostUpdate: func() {
				log.Info("Seed hosts updated", "hosts", seedHosts)
				annotation.MarkPodsAsUpdated(c, es)
			},
		})
}
