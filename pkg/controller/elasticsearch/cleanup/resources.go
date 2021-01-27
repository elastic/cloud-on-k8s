// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cleanup

import (
	"context"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = ulog.Log.WithName("cleanup")

// DeleteAfter represents how long after creation an object can be safely garbage collected.
const DeleteAfter = 10 * time.Minute

// IsTooYoungForGC checks the object creation time, and returns true
// if we consider it should not be garbage collected yet.
// This is to avoid situations where we would delete an object that was just created,
// or delete an object due to a (temporary) out-of-sync cache.
func IsTooYoungForGC(object metav1.Object) bool {
	creationTime := object.GetCreationTimestamp()
	return time.Since(creationTime.Time) < DeleteAfter
}

// DeleteOrphanedSecrets cleans up secrets that are not needed anymore for the given es cluster.
func DeleteOrphanedSecrets(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) error {
	span, _ := apm.StartSpan(ctx, "delete_orphaned_secrets", tracing.SpanTypeApp)
	defer span.End()

	var secrets corev1.SecretList
	ns := client.InNamespace(es.Namespace)
	matchLabels := label.NewLabelSelectorForElasticsearch(es)
	if err := c.List(context.Background(), &secrets, ns, matchLabels); err != nil {
		return err
	}
	resources := make([]client.Object, len(secrets.Items))
	for i := range secrets.Items {
		resources[i] = &secrets.Items[i]
	}
	return cleanupFromPodReference(c, es.Namespace, resources)
}

// cleanupFromPodReference deletes objects having a reference to
// a pod which does not exist anymore.
func cleanupFromPodReference(c k8s.Client, namespace string, objects []client.Object) error {
	for _, runtimeObj := range objects {
		obj, err := meta.Accessor(runtimeObj)
		if err != nil {
			return err
		}
		podName, hasPodReference := obj.GetLabels()[label.PodNameLabelName]
		if !hasPodReference {
			continue
		}
		// this secret applies to a particular pod
		// remove it if the pod does not exist anymore
		var pod corev1.Pod
		err = c.Get(context.Background(), types.NamespacedName{
			Namespace: namespace,
			Name:      podName,
		}, &pod)
		if apierrors.IsNotFound(err) {
			if IsTooYoungForGC(obj) {
				// pod might not be there in the cache yet,
				// skip deletion
				continue
			}
			// pod does not exist anymore, delete the object
			log.Info("Garbage-collecting resource", "namespace", namespace, "name", obj.GetName())
			if deleteErr := c.Delete(context.Background(), runtimeObj); deleteErr != nil {
				return deleteErr
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}
