package kibana

func NewDeploymentName(stackName string) string {
	return stackName + "-kibana"
}
