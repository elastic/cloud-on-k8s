package controller

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/license"
)

func init() {
	Register(operator.LicenseOperator, license.Add)
}
