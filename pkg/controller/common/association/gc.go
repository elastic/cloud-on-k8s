// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	log = logf.Log.WithName("association")
)

// UsersGarbageCollector allows to remove unused Users. Users should be deleted as part of the association controllers
// reconciliation loop. But without a Finalizer nothing prevents the associated resource to be removed while the
// operator is not running.
// This code is intended to be run during startup, before the controllers are started, to detect and delete such
// orphaned resources.
type UsersGarbageCollector struct {
	client k8s.Client
	scheme *runtime.Scheme

	// registeredResources are resources that will be garbage collected if they are
	// detected as orphaned.
	registeredResources []registeredResource
}

type registeredResource struct {
	apiType                                         runtime.Object
	associationNameLabel, associationNamespaceLabel string
}

// NewUsersGarbageCollector creates a new UsersGarbageCollector instance.
func NewUsersGarbageCollector(cfg *rest.Config, Scheme *runtime.Scheme) (*UsersGarbageCollector, error) {
	cl, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}
	return &UsersGarbageCollector{
		client: k8s.WrapClient(cl),
		scheme: Scheme,
	}, nil
}

// For is used to register the associated resources and the annotation names needed to resolve the name
// of the associated resource.
func (ugc *UsersGarbageCollector) For(
	apiType runtime.Object,
	associationNamespaceLabel, associationNameLabel string,
) *UsersGarbageCollector {
	ugc.registeredResources = append(ugc.registeredResources,
		registeredResource{
			apiType:                   apiType,
			associationNameLabel:      associationNameLabel,
			associationNamespaceLabel: associationNamespaceLabel},
	)
	return ugc
}

func (ugc *UsersGarbageCollector) getUserSecrets() ([]v1.Secret, error) {
	userSecrets := v1.SecretList{}
	matchLabels := client.MatchingLabels(map[string]string{
		common.TypeLabelName: user.UserType,
	})
	if err := ugc.client.List(&userSecrets, matchLabels); err != nil {
		return nil, err
	}
	return userSecrets.Items, nil
}

// DoGarbageCollection runs the User garbage collector.
func (ugc *UsersGarbageCollector) DoGarbageCollection() error {

	// Shortcut execution if there's no resources to garbage collect
	if len(ugc.registeredResources) == 0 {
		return nil
	}

	// 1. List all secrets of type "user"
	secrets, err := ugc.getUserSecrets()
	if err != nil {
		return err
	}
	if len(secrets) == 0 {
		return nil
	}

	// 2. List all parent resources. We retrieve *all* the registered resources in order to reduce the amount of
	// API calls. The tradeoff here is the memory that is temporarily consumed during the garbage collection phase.
	allParents, err := ugc.listAssociatedResources()
	if err != nil {
		return err
	}

	for _, secret := range secrets {
		if apiType, expectedResource, match := ugc.matchRegisteredResource(secret); match {
			nns, ok := allParents[*apiType]
			if !ok {
				continue
			}
			_, found := nns[expectedResource]
			if !found {
				log.Info("Deleting orphaned user secret", "namespace", secret.Namespace, "secret_name", secret.Name)
				err = ugc.client.Delete(&secret)
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
			return nil, err
		}
		if !(strings.HasSuffix(gvk.Kind, "List") && meta.IsListType(resource.apiType)) {
			return nil, fmt.Errorf("non-list type %T (kind %q) passed as input", resource.apiType, gvk)
		}

		list := resource.apiType.DeepCopyObject()
		err = ugc.client.List(list)
		if err != nil {
			return nil, err
		}
		objects, err := meta.ExtractList(list)
		if err != nil {
			return nil, err
		}

		for _, obj := range objects {
			accessor, err := meta.Accessor(obj)
			if err != nil {
				return nil, err
			}
			resources[types.NamespacedName{
				Namespace: accessor.GetNamespace(),
				Name:      accessor.GetName(),
			}] = struct{}{}
		}

	}

	return result, nil
}
