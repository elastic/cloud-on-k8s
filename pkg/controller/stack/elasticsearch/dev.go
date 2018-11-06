package elasticsearch

import (
	"os"
	"os/exec"
	"strings"
)

// ExternalServiceURL is the environment specific public Elasticsearch URL.
func ExternalServiceURL(stackName string) (string, error) {
	if useMinikube() {
		url, err := getMinikubeServiceURL(PublicServiceName(stackName))
		if err != nil {
			return "", err
		}
		return url, nil
	}
	return PublicServiceURL(stackName), nil
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
