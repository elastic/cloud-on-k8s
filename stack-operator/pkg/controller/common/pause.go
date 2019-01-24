package common

import (
	"context"
	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"time"
)

const (
	// PauseAnnotationName annotation
	PauseAnnotationName = "common.k8s.elastic.co/pause"
)

var (
	stack        = reflect.TypeOf(deploymentsv1alpha1.Stack{}).Name()

	// PauseRequeue is the default requeue result if controller is paused
	PauseRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// IsPaused computes if a given controller is paused.
func IsPaused(meta v1.ObjectMeta, client client.Client) bool {
	return getBoolFromAnnotation(meta.Annotations) || IsStackOwnerPaused(meta.Namespace, meta.OwnerReferences, client)
}

// IsStackOwnerPaused checks if the parent Stack is paused.
func IsStackOwnerPaused(namespace string, owners []v1.OwnerReference, client client.Client) bool {
	// Check if annotation is set on owner.
	for _, owner := range owners {
		if owner.Kind == stack {
			var stackInstance deploymentsv1alpha1.Stack
			name := types.NamespacedName{Namespace: namespace, Name: owner.Name}
			if err := client.Get(context.TODO(), name, &stackInstance); err != nil {
				log.Error(err, "Cannot retrieve stack instance")
				return false
			}
			return getBoolFromAnnotation(stackInstance.Annotations)
		}
	}
	return false
}

// Extract the desired state from the map that contains annotations.
func getBoolFromAnnotation(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	stateAsString, exists := annotations[PauseAnnotationName]

	if !exists {
		return false
	}

	expectedState, err := strconv.ParseBool(stateAsString)
	if err != nil {
		log.Error(err, "Cannot parse %s as a bool, defaulting to %s: \"false\"", annotations[PauseAnnotationName], PauseAnnotationName)
		return false
	}

	return expectedState
}
