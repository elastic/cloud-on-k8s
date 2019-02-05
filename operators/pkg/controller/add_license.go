package controller

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/license"
)

func init() {
	Register(operator.LicenseOperator, license.Add)
}
