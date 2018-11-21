package kibana

import "github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"

func NewDeploymentName(stackName string) string {
	return common.Concat(stackName, "-kibana")
}
