package elasticsearch

import (
	"os"
	"os/exec"
)

func ExternalServiceURL(stackName string) string {
	if IsRunningInKubernetes() {
		return PublicServiceURL(stackName)
	}
	url, err := GetDevServiceURL(PublicServiceName(stackName))
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

func GetDevServiceURL(service string) (string, error) {
	res, err := exec.Command("minikube", "service", "--url", service).Output()
	return string(res[:(len(res) - 1)]), err //drop newline at the end
}
