// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	log = logf.Log.WithName("association")
)

const APIBasePath = "/apis"

type clientFactory func(baseConfig *rest.Config, gv schema.GroupVersion) (rest.Interface, error)

// UsersGarbageCollector allows to remove unused Users. Users should be deleted as part of the association controllers
// reconciliation loop. But without a Finalizer nothing prevent the associated resource to be removed while the
// operator is not running.
// This code is intended to be run during startup, before the controllers are started, to detect and delete such
// orphaned resources.
type UsersGarbageCollector struct {
	// clientset is used to list potential orphaned secrets
	clientset kubernetes.Interface

	// clientFactory provides a REST client for a given resource
	clientFactory clientFactory

	baseConfig *rest.Config

	mapper meta.RESTMapper
	scheme *runtime.Scheme

	// registeredResources are resources that will be garbage collect of they are
	// detected as orphaned.
	registeredResources []registeredResource
}

type registeredResource struct {
	apiType                                         runtime.Object
	associationNameLabel, associationNamespaceLabel string
}

// NewUsersGarbageCollector creates a new UsersGarbageCollector instance.
func NewUsersGarbageCollector(clientset kubernetes.Interface, cfg *rest.Config, scheme *runtime.Scheme) (*UsersGarbageCollector, error) {
	mapper, err := apiutil.NewDiscoveryRESTMapper(cfg)
	if err != nil {
		return nil, err
	}

	return &UsersGarbageCollector{
		clientset:     clientset,
		clientFactory: newClientFor,
		baseConfig:    cfg,
		mapper:        mapper,
		scheme:        scheme,
	}, nil
}

// RegisterForUserGC is used to register the associated resources and the annotation names needed to resolve the name
// of the associated resource.
func (ugc *UsersGarbageCollector) RegisterForUserGC(
	apiType runtime.Object,
	associationNamespaceLabel, associationNameLabel string,
) {
	ugc.registeredResources = append(ugc.registeredResources,
		registeredResource{
			apiType:                   apiType,
			associationNameLabel:      associationNameLabel,
			associationNamespaceLabel: associationNamespaceLabel},
	)
}

// GC runs the User garbage collector.
func (ugc *UsersGarbageCollector) GC() error {

	// Shortcut execution if there's no resources to garbage collect
	if len(ugc.registeredResources) == 0 {
		return nil
	}

	// 1. List all secrets of type "user"
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{common.TypeLabelName: user.UserType},
	}
	secrets, err := ugc.clientset.CoreV1().Secrets("").List(metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	})
	if err != nil {
		return err
	}

	// 2. List all parent resources. We retrieve *all* the registered resources in order to reduce the amount of
	// API calls. The tradeoff here is the memory that is temporarily consumed during the garbage collection phase.
	registeredResources, err := ugc.listAssociatedResources()
	if err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		if apiType, expectedResource, match := ugc.matchRegisteredResource(secret); match {
			nns, ok := registeredResources[*apiType]
			if !ok {
				continue
			}
			_, found := nns[expectedResource]
			if !found {
				log.Info("Found orphaned user secret", "namespace", secret.Namespace, "secret_name", secret.Name)
				err = ugc.clientset.CoreV1().Secrets(secret.Namespace).Delete(secret.Name, &metav1.DeleteOptions{})
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}
	return nil
}

type resourcesByAPIType map[runtime.Object]map[types.NamespacedName]struct{}

// matchRegisteredResource checks if a User secret belongs to an associated resource using the Secret's annotations.
// If it matches then it returns the type (e.g. APM Server or Kibana) and the name of the associated resource.
func (ugc *UsersGarbageCollector) matchRegisteredResource(secret v1.Secret) (*runtime.Object, types.NamespacedName, bool) {
	for _, resource := range ugc.registeredResources {
		namespace, ok := secret.Labels[resource.associationNamespaceLabel]
		if !ok {
			continue
		}
		name, ok := secret.Labels[resource.associationNameLabel]
		if !ok {
			continue
		}
		return &resource.apiType, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, true
	}
	return nil, types.NamespacedName{}, false
}

// listAssociatedResources gets the list of all the instances of the registered resources.
func (ugc *UsersGarbageCollector) listAssociatedResources() (resourcesByAPIType, error) {
	result := make(resourcesByAPIType)

	for _, resource := range ugc.registeredResources {
		resources := make(map[types.NamespacedName]struct{})
		result[resource.apiType] = resources
		gvk, err := apiutil.GVKForObject(resource.apiType, ugc.scheme)
		if err != nil {
			return result, err
		}
		if !(strings.HasSuffix(gvk.Kind, "List") && meta.IsListType(resource.apiType)) {
			return result, fmt.Errorf("non-list type %T (kind %q) passed as input", resource.apiType, gvk)
		}
		gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]

		client, err := ugc.clientFactory(ugc.baseConfig, gvk.GroupVersion())
		if err != nil {
			return result, err
		}

		mapping, err := ugc.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return result, err
		}
		r := &metav1.PartialObjectMetadataList{}
		err = client.Get().Resource(mapping.Resource.Resource).Do().Into(r)
		if err != nil {
			return result, err
		}
		for _, metadata := range r.Items {
			resources[types.NamespacedName{
				Namespace: metadata.Namespace,
				Name:      metadata.Name,
			}] = struct{}{}
		}
	}

	return result, nil
}

// newClientFor returns a rest client to access a given resource
func newClientFor(baseConfig *rest.Config, gv schema.GroupVersion) (rest.Interface, error) {
	cfg := rest.CopyConfig(baseConfig)
	cfg.ContentConfig.GroupVersion = &gv
	cfg.APIPath = APIBasePath
	cfg.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}
	cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	return rest.RESTClientFor(cfg)
}
