// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	AllNamespaces = ""
)

// UsersGarbageCollector allows to remove unused users. Users should be deleted as part of the association controllers
// reconciliation loop. But without a Finalizer nothing prevents the associated resource to be removed while the
// operator is not running.
// This code is intended to be run during startup, before the controllers are started, to detect and delete such
// orphaned resources.
type UsersGarbageCollector struct {
	client            k8s.Client
	managedNamespaces []string

	// registeredResources are resources that will be garbage collected if they are
	// detected as orphaned.
	registeredResources []registeredResource
}

type registeredResource struct {
	apiType                                         client.ObjectList
	associationNameLabel, associationNamespaceLabel string
}

// NewUsersGarbageCollector creates a new UsersGarbageCollector instance.
func NewUsersGarbageCollector(cfg *rest.Config, managedNamespaces []string) (*UsersGarbageCollector, error) {
	// Use a sync client here in order not to depend on any cache initialization
	cl, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}
	if len(managedNamespaces) == 0 {
		managedNamespaces = []string{AllNamespaces}
	}
	return &UsersGarbageCollector{
		client:            cl,
		managedNamespaces: managedNamespaces,
	}, nil
}

// For is used to register the associated resources and the annotation names needed to resolve the name
// of the associated resource.
func (ugc *UsersGarbageCollector) For(
	apiType client.ObjectList,
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
	userSecrets := make([]v1.Secret, 0)
	for _, namespace := range ugc.managedNamespaces {
		userSecretsInNamespace, err := getUserSecretsInNamespace(ugc.client, namespace)
		if err != nil {
			return nil, err
		}
		userSecrets = append(userSecrets, userSecretsInNamespace...)
	}
	return userSecrets, nil
}

func getUserSecretsInNamespace(c k8s.Client, namespace string) ([]v1.Secret, error) {
	userSecrets := v1.SecretList{}
	userLabels := client.MatchingLabels(map[string]string{common.TypeLabelName: esuser.AssociatedUserType})
	if err := c.List(context.Background(), &userSecrets, client.InNamespace(namespace), userLabels); err != nil {
		return nil, err
	}

	serviceAccountSecrets := v1.SecretList{}
	serviceAccountLabels := client.MatchingLabels(map[string]string{common.TypeLabelName: esuser.ServiceAccountTokenType})
	if err := c.List(context.Background(), &serviceAccountSecrets, client.InNamespace(namespace), serviceAccountLabels); err != nil {
		return nil, err
	}

	secrets := append(userSecrets.Items, serviceAccountSecrets.Items...)
	return secrets, nil
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
		secret := secret
		if apiType, expectedParent, hasParent := ugc.getAssociationParent(secret); hasParent {
			parents, ok := allParents[*apiType] // get all the parents of a given type
			if !ok {
				continue
			}
			_, found := parents[expectedParent]
			if !found {
				log.Info("Deleting orphaned user secret", "namespace", secret.Namespace, "secret_name", secret.Name)
				err = ugc.client.Delete(context.Background(), &secret)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}
	return nil
}

type resourcesByAPIType map[runtime.Object]map[types.NamespacedName]struct{}

// getAssociationParent checks if a User secret belongs to an associated resource using the Secret's annotations.
// If it matches then it returns the type (e.g. APM Server or Kibana) and the name of the associated resource.
func (ugc *UsersGarbageCollector) getAssociationParent(secret v1.Secret) (*client.ObjectList, types.NamespacedName, bool) {
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
		objects, err := ugc.getResourcesInNamespaces(resource.apiType)
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

func (ugc *UsersGarbageCollector) getResourcesInNamespaces(apiType client.ObjectList) ([]runtime.Object, error) {
	objects := make([]runtime.Object, 0)
	for _, namespace := range ugc.managedNamespaces {
		list := apiType.DeepCopyObject().(client.ObjectList) //nolint:forcetypeassert
		err := ugc.client.List(context.Background(), list, client.InNamespace(namespace))
		if err != nil {
			return nil, err
		}
		objectsInNamespace, err := meta.ExtractList(list)
		if err != nil {
			return nil, err
		}
		objects = append(objects, objectsInNamespace...)
	}
	return objects, nil
}
