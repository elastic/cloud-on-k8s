package controller

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack"
)

func init() {
	Register(operator.ApplicationOperator, stack.Add)
}
