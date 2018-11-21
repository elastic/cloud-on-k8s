package elasticsearch

import (
	"os"
	"os/exec"
	"strings"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
)

// ExternalServiceURL is the environment specific public Elasticsearch URL.
func ExternalServiceURL(stack deploymentsv1alpha1.Stack) (string, error) {
	if useMinikube() {
		url, err := getMinikubeServiceURL(PublicServiceName(stack.Name))
		if err != nil {
			return "", err
		}
		return url, nil
	}
	return PublicServiceURL(stack), nil
}

func useMinikube() bool {
	return len(os.Getenv("USE_MINIKUBE")) > 0
}

func getMinikubeServiceURL(service string) (string, error) {
	res, err := exec.Command("minikube", "service", "--url", service).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(res), "\n"), err
}
