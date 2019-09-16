package version

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
)

type AddedRemoved bool

const (
	Added   AddedRemoved = true
	Removed AddedRemoved = true
)

type ZenConfigManager struct {
	es     v1alpha1.Elasticsearch
	client k8s.Client
}

type MasterMutation struct {
	name     string          // The name of the master
	version  version.Version // The version of the master added or removed
	mutation AddedRemoved    // Boolean used to described if it is added or remove
}

func (z ZenConfigManager) PrepareForMutation(
	resources nodespec.Resources, // used to update the configuration in case of Zen1
	actualMasters corev1.Pod, // used to know if we should take care of Zen1 or Zen2
	mutation AddedRemoved,
) {

}
