// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"encoding/json"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
)

const (
	RemoteClustersAnnotationName = "elasticsearch.k8s.elastic.co/remote-clusters"
)

type remoteClusterState struct {
	Name       string `json:"name"`
	ConfigHash string `json:"configHash"`
}

type expectedRemoteClusterConfiguration struct {
	remoteClusterState
	esv1.K8sLocalRemoteCluster
}

// getCurrentRemoteClusters returns a map with the current configuration hash of the remote clusters declared in Elasticsearch.
// A map is returned here to quickly compare with the ones that are new or missing.
func getCurrentRemoteClusters(es esv1.Elasticsearch) (map[string]string, error) {
	serializedRemoteClusters, ok := es.Annotations[RemoteClustersAnnotationName]
	if !ok {
		return nil, nil
	}
	var remoteClustersArray []remoteClusterState
	if err := json.Unmarshal([]byte(serializedRemoteClusters), &remoteClustersArray); err != nil {
		return nil, err
	}

	remoteClusters := make(map[string]string)
	for _, remoteCluster := range remoteClustersArray {
		remoteClusters[remoteCluster.Name] = remoteCluster.ConfigHash
	}

	return remoteClusters, nil
}

func annotateWithRemoteClusters(c k8s.Client, es esv1.Elasticsearch, remoteClusters map[string]expectedRemoteClusterConfiguration) error {
	// We don't need to store the map in the annotation
	remoteClustersList := make([]remoteClusterState, len(remoteClusters))
	i := 0
	for _, remoteCluster := range remoteClusters {
		remoteClustersList[i] = remoteClusterState{
			Name:       getRemoteClusterKey(remoteCluster.ElasticsearchRef),
			ConfigHash: hash.HashObject(remoteCluster.K8sLocalRemoteCluster),
		}
		i++
	}
	// serialize the remote clusters list and update the object
	serializedRemoteClusters, err := json.Marshal(remoteClustersList)
	if err != nil {
		return errors.Wrapf(err, "failed to serialize configuration")
	}

	if es.Annotations == nil {
		es.Annotations = make(map[string]string)
	}

	es.Annotations[RemoteClustersAnnotationName] = string(serializedRemoteClusters)
	return c.Update(&es)
}
