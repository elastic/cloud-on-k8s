package version

import (
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	// ElasticsearchVersionLabelName is the name of the label that contains the Elasticsearch version of the resource.
	ElasticsearchVersionLabelName = "elasticsearch.stack.k8s.elastic.co/version"
)

var (
	log = logf.Log.WithName("version")
)
