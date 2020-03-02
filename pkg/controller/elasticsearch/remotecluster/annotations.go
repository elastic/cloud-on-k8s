// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"encoding/json"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
)

const (
	RemoteClustersAnnotationName = "elasticsearch.k8s.elastic.co/remote-clusters"
)

type expectedRemoteClusterConfiguration struct {
	esv1.RemoteCluster

	// ConfigHash is the hash of the remote cluster configuration. It is used to detect when the settings must be updated.
	ConfigHash string `json:"configHash"`
}

// getCurrentRemoteClusters returns a map with the current configuration hash of the remote clusters declared in Elasticsearch.
// A map is returned here to quickly compare with the ones that are new or missing.
// If there's no remote clusters the map is empty but not nil.
func getCurrentRemoteClusters(es esv1.Elasticsearch) (map[string]string, error) {
	serializedRemoteClusters, ok := es.Annotations[RemoteClustersAnnotationName]
	remoteClusters := make(map[string]string)
	if !ok {
		return remoteClusters, nil
	}
	if err := json.Unmarshal([]byte(serializedRemoteClusters), &remoteClusters); err != nil {
		return nil, err
	}

	return remoteClusters, nil
}

func annotateWithRemoteClusters(c k8s.Client, es esv1.Elasticsearch, remoteClusters map[string]expectedRemoteClusterConfiguration) error {
	// Store a map with the configuration hash for every remote cluster
	remoteClusterConfigurations := make(map[string]string, len(remoteClusters))
	for _, remoteCluster := range remoteClusters {
		// remoteCluster.Name is set by the user, it is supposed to be unique
		remoteClusterConfigurations[remoteCluster.Name] = remoteCluster.RemoteCluster.ConfigHash()
	}

	// serialize the remote clusters list and update the object
	serializedRemoteClusters, err := json.Marshal(remoteClusterConfigurations)
	if err != nil {
		return errors.Wrapf(err, "failed to serialize configuration")
	}

	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}

	es.Annotations[RemoteClustersAnnotationName] = string(serializedRemoteClusters)
	return c.Update(&es)
}
