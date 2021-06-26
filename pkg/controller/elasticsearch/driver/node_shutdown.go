package driver

import (
	"github.com/blang/semver/v4"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/shutdown"
)


func newShutdownInterface(es esv1.Elasticsearch) (shutdown.Interface, error) {
	v, err := semver.Parse(es.Spec.Version) 
	if err != nil {
return nil, err
	}
	if v.GTE(semver.MustParse("7.14.0-SNAPSHOT")

}
