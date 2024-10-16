// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remotecluster

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/certificates/transport"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
)

// -- Fake ES client

type fakeESClient struct {
	esclient.Client

	// fake responses
	getCrossClusterAPIKeys []string

	// recorded requests
	invalidateCrossClusterAPIKey     []string
	crossClusterAPIKeyCreateRequests []esclient.CrossClusterAPIKeyCreateRequest
	existingCrossClusterAPIKeys      esclient.CrossClusterAPIKeyList
	updateCrossClusterAPIKey         map[string]esclient.CrossClusterAPIKeyUpdateRequest
}

func (f *fakeESClient) CreateCrossClusterAPIKey(ctx context.Context, crossClusterAPIKeyCreateRequest esclient.CrossClusterAPIKeyCreateRequest) (esclient.CrossClusterAPIKeyCreateResponse, error) {
	f.crossClusterAPIKeyCreateRequests = append(f.crossClusterAPIKeyCreateRequests, crossClusterAPIKeyCreateRequest)
	return esclient.CrossClusterAPIKeyCreateResponse{
		ID:      fmt.Sprintf("generated-id-from-fake-es-client-%s", crossClusterAPIKeyCreateRequest.Name),
		Name:    crossClusterAPIKeyCreateRequest.Name,
		Encoded: fmt.Sprintf("generated-encoded-key-from-fake-es-client-for-%s", crossClusterAPIKeyCreateRequest.Name),
	}, nil
}

func (f *fakeESClient) GetCrossClusterAPIKeys(_ context.Context, name string) (esclient.CrossClusterAPIKeyList, error) {
	f.getCrossClusterAPIKeys = append(f.getCrossClusterAPIKeys, name)
	return f.existingCrossClusterAPIKeys, nil
}

func (f *fakeESClient) InvalidateCrossClusterAPIKey(_ context.Context, name string) error {
	f.invalidateCrossClusterAPIKey = append(f.invalidateCrossClusterAPIKey, name)
	return nil
}

func (f *fakeESClient) UpdateCrossClusterAPIKey(_ context.Context, name string, updateRequest esclient.CrossClusterAPIKeyUpdateRequest) (esclient.CrossClusterAPIKeyUpdateResponse, error) {
	if f.updateCrossClusterAPIKey == nil {
		f.updateCrossClusterAPIKey = make(map[string]esclient.CrossClusterAPIKeyUpdateRequest)
	}
	f.updateCrossClusterAPIKey[name] = updateRequest
	return esclient.CrossClusterAPIKeyUpdateResponse{}, nil
}

// -- Fake cluster builder

type clusterBuilder struct {
	name, namespace, version string
	remoteClusters           []esv1.RemoteCluster
}

func newClusterBuilder(namespace, name, version string) *clusterBuilder {
	return &clusterBuilder{
		name:      name,
		namespace: namespace,
		version:   version,
	}
}

func (cb *clusterBuilder) withRemoteCluster(namespace, name string) *clusterBuilder {
	cb.remoteClusters = append(cb.remoteClusters,
		esv1.RemoteCluster{
			Name: fmt.Sprintf("alias-from-%s-%s-to-%s-%s", cb.namespace, cb.name, namespace, name),
			ElasticsearchRef: commonv1.LocalObjectSelector{
				Name:      name,
				Namespace: namespace,
			},
		})
	return cb
}

func (cb *clusterBuilder) withAPIKey(namespace, name string, apiKey *esv1.RemoteClusterAPIKey) *clusterBuilder {
	cb.remoteClusters = append(cb.remoteClusters,
		esv1.RemoteCluster{
			Name: fmt.Sprintf("generated-alias-from-%s-%s-to-%s-%s-with-api-key", cb.namespace, cb.name, namespace, name),
			ElasticsearchRef: commonv1.LocalObjectSelector{
				Name:      name,
				Namespace: namespace,
			},
			APIKey: apiKey,
		})
	return cb
}

func (cb *clusterBuilder) build() []client.Object {
	remoteClusters := make([]esv1.RemoteCluster, len(cb.remoteClusters))
	for i, remoteCluster := range cb.remoteClusters {
		remoteCluster := remoteCluster.DeepCopy()
		remoteClusters[i] = *remoteCluster
	}
	return []client.Object{
		&esv1.Elasticsearch{
			ObjectMeta: v1.ObjectMeta{
				Namespace: cb.namespace,
				Name:      cb.name,
			},
			Spec: esv1.ElasticsearchSpec{
				Version:        cb.version,
				RemoteClusters: remoteClusters,
			},
			Status: esv1.ElasticsearchStatus{
				AvailableNodes: 3,
				Version:        cb.version,
			},
		},
		&corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Namespace: cb.namespace,
				Name:      fmt.Sprintf("es-%s-%s-1", cb.namespace, cb.name),
				Labels: map[string]string{
					label.ClusterNameLabelName: cb.name,
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
	}
}

type fakeAccessReviewer struct {
	allowed bool
	err     error
}

func (f *fakeAccessReviewer) AccessAllowed(_ context.Context, _ string, _ string, _ runtime.Object) (bool, error) {
	return f.allowed, f.err
}

func fakePublicCa(namespace, name string) *corev1.Secret {
	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	transportPublicCertKey := transport.PublicCertsSecretRef(namespacedName)
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: transportPublicCertKey.Namespace,
			Name:      transportPublicCertKey.Name,
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte(namespacedName.String()),
		},
	}
}

// remoteCa builds an expected remote Ca
func remoteCa(localNamespace, localName, remoteNamespace, remoteName string) *corev1.Secret {
	remoteNamespacedName := types.NamespacedName{
		Name:      remoteName,
		Namespace: remoteNamespace,
	}
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: localNamespace,
			Name:      remoteCASecretName(localName, remoteNamespacedName),
			Labels: map[string]string{
				"common.k8s.elastic.co/type":                            "remote-ca",
				"elasticsearch.k8s.elastic.co/cluster-name":             localName,
				"elasticsearch.k8s.elastic.co/remote-cluster-name":      remoteName,
				"elasticsearch.k8s.elastic.co/remote-cluster-namespace": remoteNamespace,
			},
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte(remoteNamespacedName.String()),
		},
	}
}

func withDataCert(caSecret *corev1.Secret, newCa []byte) *corev1.Secret {
	caSecret.Data[certificates.CAFileName] = newCa
	return caSecret
}
