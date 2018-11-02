package stack

// StackID returns the qualified identifier for this stack deployment
// based on the given namespace and stack name, following
// the convention: <namespace>-<stack name>
func StackID(namespace string, stackName string) string {
	return namespace + "-" + stackName
}
