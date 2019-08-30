package interfaces

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AssociationStatus is the status of an association resource.
type AssociationStatus string

const (
	AssociationUnknown     AssociationStatus = ""
	AssociationPending     AssociationStatus = "Pending"
	AssociationEstablished AssociationStatus = "Established"
	AssociationFailed      AssociationStatus = "Failed"
)

// Associated interface represents a Elastic stack application that is associated with an Elasticsearch cluster.
// An associated object needs some credentials to establish a connection to the Elasticsearch cluster and usually it
// offers a keystore which in ECK is represented with an underlying Secret.
// Kibana and the APM server are two examples of associated objects.
type Associated interface {
	metav1.Object
	runtime.Object
	ElasticsearchAuth() commonv1alpha1.ElasticsearchAuth
	ElasticsearchRef() commonv1alpha1.ObjectSelector
}
