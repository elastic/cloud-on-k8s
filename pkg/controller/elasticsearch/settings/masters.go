// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"context"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
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
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	pods []corev1.Pod,
) error {
	span, ctx := apm.StartSpan(ctx, "update_seed_hosts", tracing.SpanTypeApp)
	defer span.End()
	log := ulog.FromContext(ctx)

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
				net.JoinHostPort(master.Status.PodIP, strconv.Itoa(network.TransportPort)),
			)
		}
	}

	var hosts string
	if seedHosts != nil {
		// avoid unnecessary config map updates due to changing order of seed hosts
		sort.Strings(seedHosts)
		hosts = strings.Join(seedHosts, "\n")
	}
	expected := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.UnicastHostsConfigMap(es.Name),
			Namespace: es.Namespace,
			Labels:    label.NewLabels(k8s.ExtractNamespacedName(&es)),
		},
		Data: map[string]string{
			volume.UnicastHostsFile: hosts,
		},
	}

	reconciled := &corev1.ConfigMap{}
	return reconciler.ReconcileResource(
		reconciler.Params{
			Context:    ctx,
			Client:     c,
			Owner:      &es,
			Expected:   &expected,
			Reconciled: reconciled,
			NeedsUpdate: func() bool {
				return !reflect.DeepEqual(expected.Data, reconciled.Data)
			},
			UpdateReconciled: func() {
				reconciled.Data = expected.Data
			},
			PreCreate: func() error {
				log.Info("Creating seed hosts", "namespace", es.Namespace, "es_name", es.Name, "hosts", seedHosts)
				return nil
			},
			PostUpdate: func() {
				log.Info("Seed hosts updated", "namespace", es.Namespace, "es_name", es.Name, "hosts", seedHosts)
				annotation.MarkPodsAsUpdated(ctx, c,
					client.InNamespace(es.Namespace),
					label.NewLabelSelectorForElasticsearch(es))
			},
		})
}
