package controller

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch"
)

func init() {
	Register(operator.ApplicationOperator, elasticsearch.Add)
}
