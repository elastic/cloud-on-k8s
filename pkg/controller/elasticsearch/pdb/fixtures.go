// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
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
func (b Builder) WithNodeSet(name string, count int32, nodeTypes ...string) Builder {
	config := map[string]interface{}{}

	// Convert legacy node type notation to roles array
	if len(nodeTypes) > 0 {
		roles := []string{}
		for _, nodeType := range nodeTypes {
			// Convert legacy node.X format to just X
			if role := strings.TrimPrefix(nodeType, "node."); role != nodeType {
				roles = append(roles, role)
			}
		}

		// Only set roles if we have any
		if len(roles) > 0 {
			config["node.roles"] = roles
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
func (b Builder) buildStatefulSet(name string, replicas int32, nodeTypes []string) appsv1.StatefulSet {
	sset := statefulset.TestSset{
		Namespace:   b.Elasticsearch.Namespace,
		Name:        name,
		ClusterName: b.Elasticsearch.Name,
		Version:     b.Elasticsearch.Spec.Version,
		Replicas:    replicas,
	}

	// Set node roles based on nodeTypes
	for _, nodeType := range nodeTypes {
		// Strip the "node." prefix if present
		role := strings.TrimPrefix(nodeType, "node.")

		switch role {
		case "master":
			sset.Master = true
		case "data":
			sset.Data = true
		case "ingest":
			sset.Ingest = true
		case "ml":
			sset.ML = true
		case "transform":
			sset.Transform = true
		case "remote_cluster_client":
			sset.RemoteClusterClient = true
		case "data_hot":
			sset.DataHot = true
		case "data_warm":
			sset.DataWarm = true
		case "data_cold":
			sset.DataCold = true
		case "data_content":
			sset.DataContent = true
		case "data_frozen":
			sset.DataFrozen = true
		}
	}

	return sset.Build()
}

// WithStatefulSet adds a custom StatefulSet to the builder.
func (b Builder) WithStatefulSet(sset appsv1.StatefulSet) Builder {
	b.StatefulSets = append(b.StatefulSets, sset)
	return b
}

// BuildResourcesList generates a nodespec.ResourcesList from the builder data.
func (b Builder) BuildResourcesList() (nodespec.ResourcesList, error) {
	v, err := version.Parse(b.Elasticsearch.Spec.Version)
	if err != nil {
		return nil, err
	}

	resourcesList := make(nodespec.ResourcesList, 0, len(b.StatefulSets))

	for i, sset := range b.StatefulSets {
		// Create config based on the nodeset if available
		var config *v1.Config
		if i < len(b.Elasticsearch.Spec.NodeSets) {
			config = b.Elasticsearch.Spec.NodeSets[i].Config
		} else {
			config = &v1.Config{Data: map[string]interface{}{}}
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

// WithMasterDataNodes adds both master and data nodes to the Elasticsearch cluster.
func (b Builder) WithMasterDataNodes(name string, count int32) Builder {
	return b.WithNodeSet(name, count, "node.master", "node.data")
}

// WithMasterOnlyNodes adds master-only nodes to the Elasticsearch cluster.
func (b Builder) WithMasterOnlyNodes(name string, count int32) Builder {
	return b.WithNodeSet(name, count, "node.master")
}

// WithDataOnlyNodes adds data-only nodes to the Elasticsearch cluster.
func (b Builder) WithDataOnlyNodes(name string, count int32) Builder {
	return b.WithNodeSet(name, count, "node.data")
}

// WithIngestOnlyNodes adds ingest-only nodes to the Elasticsearch cluster.
func (b Builder) WithIngestOnlyNodes(name string, count int32) Builder {
	return b.WithNodeSet(name, count, "node.ingest")
}

func (b Builder) GetStatefulSets() []appsv1.StatefulSet {
	return b.StatefulSets
}
