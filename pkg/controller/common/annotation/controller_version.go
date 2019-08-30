// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package annotation

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ControllerVersionAnnotation is the annotation name that indicates the last controller version to update a resource
const ControllerVersionAnnotation = "common.k8s.elastic.co/controller-version"

// UpdateControllerVersion updates the controller version annotation to the current version if necessary
func UpdateControllerVersion(client k8s.Client, obj runtime.Object, version string) error {
	accessor := meta.NewAccessor()
	namespace, err := accessor.Namespace(obj)
	if err != nil {
		log.Error(err, "error getting namespace", "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return err
	}
	name, err := accessor.Name(obj)
	if err != nil {
		log.Error(err, "error getting name", "namespace", namespace, "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return err
	}
	annotations, err := accessor.Annotations(obj)
	if err != nil {
		log.Error(err, "error getting annotations", "namespace", namespace, "name", name, "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return err
	}
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// do not send unnecessary update if the value would not change
	if annotations[ControllerVersionAnnotation] == version {
		return nil
	}

	annotations[ControllerVersionAnnotation] = version
	err = accessor.SetAnnotations(obj, annotations)
	if err != nil {
		log.Error(err, "error updating controller version annotation", "namespace", namespace, "name", name, "kind", obj.GetObjectKind())
		return err
	}
	log.V(1).Info("updating controller version annotation", "namespace", namespace, "name", name, "kind", obj.GetObjectKind())
	return client.Update(obj)
}

// ReconcileCompatibility determines if this controller is compatible with a given resource by examining the controller version annotation
// controller versions 0.9.0+ cannot reconcile resources created with earlier controllers, so this lets our controller skip those resources until they can be manually recreated
// if an object does not have an annotation, it will determine if it is a new object or if it has been previously reconciled by an older controller version, as this annotation
// was not applied by earlier controller versions. it will update the object's annotations indicating it is incompatible if so
func ReconcileCompatibility(client k8s.Client, obj runtime.Object, selector map[string]string, controllerVersion string) (bool, error) {
	accessor := meta.NewAccessor()
	namespace, err := accessor.Namespace(obj)
	if err != nil {
		log.Error(err, "error getting namespace", "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return false, err
	}
	name, err := accessor.Name(obj)
	if err != nil {
		log.Error(err, "error getting name", "namespace", namespace, "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return false, err
	}
	annotations, err := accessor.Annotations(obj)
	if err != nil {
		log.Error(err, "error getting annotations", "namespace", namespace, "name", name, "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return false, err
	}

	annExists := annotations != nil && annotations[ControllerVersionAnnotation] != ""

	// if the annotation does not exist, it might indicate it was reconciled by an older controller version that did not add the version annotation,
	// in which case it is incompatible with the current controller, or it is a brand new resource that has not been reconciled by any controller yet
	if !annExists {
		exist, err := checkExistingResources(client, obj, selector)
		if err != nil {
			return false, err
		}
		if exist {
			log.Info("Resource was previously reconciled by incompatible controller version and missing annotation, adding annotation", "controller_version", controllerVersion, "namespace", namespace, "name", name, "kind", obj.GetObjectKind().GroupVersionKind().Kind)
			err = UpdateControllerVersion(client, obj, "0.8.0-UNKNOWN")
			return false, err
		}
		// no annotation exists and there are no existing resources, so this has not previously been reconciled
		err = UpdateControllerVersion(client, obj, controllerVersion)
		return true, err
	}

	currentVersion, err := version.Parse(annotations[ControllerVersionAnnotation])
	if err != nil {
		return false, errors.Wrap(err, "Error parsing current version on resource")
	}
	minVersion, err := version.Parse("0.9.0-ALPHA")
	if err != nil {
		return false, errors.Wrap(err, "Error parsing minimum compatible version")
	}
	ctrlVersion, err := version.Parse(controllerVersion)
	if err != nil {
		return false, errors.Wrap(err, "Error parsing controller version")
	}

	// if the current version is gte the minimum version then they are compatible
	if currentVersion.IsSameOrAfter(*minVersion) {
		return true, nil
	}

	log.Info("Resource was created with older version of operator, will not take action", "controller_version", ctrlVersion,
		"resource_controller_version", currentVersion, "namespace", namespace, "name", name)
	return false, nil
}

// checkExistingResources returns a bool indicating if there are existing resources created for a given resource
// the labels provided must exactly match
// todo sabo do we want to keep it generic enough to allow a selector?
func checkExistingResources(client k8s.Client, obj runtime.Object, labels map[string]string) (bool, error) {

	accessor := meta.NewAccessor()
	namespace, err := accessor.Namespace(obj)
	if err != nil {
		log.Error(err, "error getting namespace", "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		return false, err
	}
	labelSelector := ctrlclient.MatchingLabels(labels)
	nsSelector := ctrlclient.InNamespace(namespace)
	// if there's no controller version annotation on the object, then we need to see maybe the object has been reconciled by an older, incompatible controller version
	// opts := ctrlclient.ListOptions{
	// 	LabelSelector: selector,
	// 	Namespace:     namespace,
	// }
	// ctrlclient.MatchingLabels()
	// this is hard because ctrlclient.ListOptions doesnt satisfy the ctrlclient.ListOption interface -- it does not define ApplyToList() and instead
	// leaves that to wrapper types such as MatchingLabels, MatchingField, and InNamespace. it might be worth making our own listoption wrapper to keep it generic?
	// there's no function to go from a ListOptions directly to something that satisfies the ListOption interface, or to just accept a random label selector
	//  instead you must use a wrapper type MatchingLabels
	// theres no function to go from a labels.Selector to a
	var svcs corev1.ServiceList
	err = client.List(&svcs, labelSelector, nsSelector)
	if err != nil {
		return false, err
	}
	// if we listed any services successfully, then we know this cluster was reconciled by an old version since any objects reconciled by a 0.9.0+ operator would have a label
	return len(svcs.Items) != 0, nil

}
