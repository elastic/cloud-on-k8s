package elasticsearch

import (
	"os"
	"os/exec"
	"strings"
)

// ExternalServiceURL is the environment specific public Elasticsearch URL.
func ExternalServiceURL(stackName string) string {
	if useMinikube() {
		url, err := getMinikubeServiceUrl(PublicServiceName(stackName))
		if err != nil {
			panic(err)
		}
		return url
	}
	return PublicServiceURL(stackName)
}

func useMinikube() bool {
	return len(os.Getenv("USE_MINIKUBE")) > 0
}

func getMinikubeServiceUrl(service string) (string, error) {
	res, err := exec.Command("minikube", "service", "--url", service).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(res), "\n"), err
}
