// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

// Builder helps create test fixtures for the Elasticsearch PDB tests.
type Builder struct {
	Elasticsearch esv1.Elasticsearch
	StatefulSets  []appsv1.StatefulSet
}

// NewBuilder creates a new Builder with default values.
func NewBuilder(name string) Builder {
	return Builder{
		Elasticsearch: esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: esv1.ElasticsearchSpec{
				Version:  "9.0.1",
				NodeSets: []esv1.NodeSet{},
			},
		},
		StatefulSets: []appsv1.StatefulSet{},
	}
}

// WithNamespace sets the namespace for the Elasticsearch resource.
func (b Builder) WithNamespace(namespace string) Builder {
	b.Elasticsearch.Namespace = namespace
	return b
}

// WithVersion sets the version for the Elasticsearch resource.
func (b Builder) WithVersion(version string) Builder {
	b.Elasticsearch.Spec.Version = version
	return b
}

// WithNodeSet adds a NodeSet to the Elasticsearch spec.
func (b Builder) WithNodeSet(name string, count int32, nodeTypes ...esv1.NodeRole) Builder {
	config := map[string]any{}

	// Only set node.Roles if the first role is not "all_roles"
	// to properly handle no roles set to equal having all roles assigned.
	if !(len(nodeTypes) == 1 && nodeTypes[0] == "all_roles") {
		// This handles the 'coordinating' role properly.
		config["node.roles"] = []esv1.NodeRole{}
		for _, nodeType := range nodeTypes {
			if string(nodeType) != "" {
				config["node.roles"] = append(config["node.roles"].([]esv1.NodeRole), nodeType) //nolint:forcetypeassert
			}
		}
	}

	nodeset := esv1.NodeSet{
		Name:  name,
		Count: count,
		Config: &v1.Config{
			Data: config,
		},
	}

	b.Elasticsearch.Spec.NodeSets = append(b.Elasticsearch.Spec.NodeSets, nodeset)

	// Create a corresponding StatefulSet
	sset := b.buildStatefulSet(name, count, nodeTypes)
	b.StatefulSets = append(b.StatefulSets, sset)

	return b
}

// buildStatefulSet creates a StatefulSet based on the given parameters.
func (b Builder) buildStatefulSet(name string, replicas int32, nodeRoles []esv1.NodeRole) appsv1.StatefulSet {
	sset := statefulset.TestSset{
		Namespace:   b.Elasticsearch.Namespace,
		Name:        name,
		ClusterName: b.Elasticsearch.Name,
		Version:     b.Elasticsearch.Spec.Version,
		Replicas:    replicas,
	}

	// Set node roles based on nodeRoles
	for _, nodeRole := range nodeRoles {
		switch nodeRole {
		case esv1.MasterRole:
			sset.Master = true
		case esv1.DataRole:
			sset.Data = true
		case esv1.IngestRole:
			sset.Ingest = true
		case esv1.MLRole:
			sset.ML = true
		case esv1.TransformRole:
			sset.Transform = true
		case esv1.RemoteClusterClientRole:
			sset.RemoteClusterClient = true
		case esv1.DataHotRole:
			sset.DataHot = true
		case esv1.DataWarmRole:
			sset.DataWarm = true
		case esv1.DataColdRole:
			sset.DataCold = true
		case esv1.DataContentRole:
			sset.DataContent = true
		case esv1.DataFrozenRole:
			sset.DataFrozen = true
		case esv1.CoordinatingRole:
			continue
		case esv1.VotingOnlyRole:
			continue
		}
	}

	// If no roles are specified (not empty, but nil)
	// this implies all roles, which is handled by
	// 'all_roles' in the tests.
	if len(nodeRoles) == 1 && nodeRoles[0] == "all_roles" {
		sset.Master = true
		sset.Data = true
		sset.Ingest = true
		sset.ML = true
		sset.Transform = true
		sset.RemoteClusterClient = true
		sset.DataHot = true
		sset.DataWarm = true
		sset.DataCold = true
		sset.DataContent = true
		sset.DataFrozen = true
	}

	return sset.Build()
}

// WithStatefulSet adds a custom StatefulSet to the builder.
func (b Builder) WithStatefulSet(sset appsv1.StatefulSet) Builder {
	b.StatefulSets = append(b.StatefulSets, sset)
	return b
}

// BuildResourcesList generates a nodespec.ResourcesList from the builder data.
// This allows the tests to properly unpack the Config object for a nodeSet
// and use the Node.Roles directly.
func (b Builder) BuildResourcesList() (nodespec.ResourcesList, error) {
	v, err := version.Parse(b.Elasticsearch.Spec.Version)
	if err != nil {
		return nil, err
	}

	resourcesList := make(nodespec.ResourcesList, 0, len(b.StatefulSets))

	for i, sset := range b.StatefulSets {
		// Create config based on the nodeset if available
		config := &v1.Config{Data: map[string]any{}}
		if i < len(b.Elasticsearch.Spec.NodeSets) {
			config = b.Elasticsearch.Spec.NodeSets[i].Config
		}

		cfg, err := settings.NewMergedESConfig(
			b.Elasticsearch.Name,
			v,
			corev1.IPv4Protocol,
			b.Elasticsearch.Spec.HTTP,
			*config,
			nil,
			false,
			false,
			false,
		)
		if err != nil {
			return nil, err
		}

		resourcesList = append(resourcesList, nodespec.Resources{
			NodeSet:     sset.Name,
			StatefulSet: sset,
			Config:      cfg,
		})
	}

	return resourcesList, nil
}

func (b Builder) GetStatefulSets() sset.StatefulSetList {
	return b.StatefulSets
}
