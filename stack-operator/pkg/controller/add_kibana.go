package controller

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/kibana"
)

func init() {
	Register(operator.ApplicationOperator, kibana.Add)
}
