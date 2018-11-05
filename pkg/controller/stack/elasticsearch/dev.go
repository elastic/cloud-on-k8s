package elasticsearch

import (
	"os"
	"os/exec"
	"strings"
)

func ExternalServiceURL(stackName string) string {
	if IsRunningInKubernetes() {
		return PublicServiceURL(stackName)
	}
	url, err := GetMinikubeServiceUrl(PublicServiceName(stackName))
	if err != nil {
		panic(err)
	}
	return url

}

func IsRunningInKubernetes() bool {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		return false
	}
	return true
}

func GetMinikubeServiceUrl(service string) (string, error) {
	res, err := exec.Command("minikube", "service", "--url", service).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(res), "\n"), err
}
