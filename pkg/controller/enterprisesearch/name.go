package enterprisesearch

import entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"

const (
	httpServiceSuffix = "http"
	configSuffix      = "config"
)

func HTTPServiceName(entName string) string {
	return entv1.Namer.Suffix(entName, httpServiceSuffix)
}

func DeploymentName(entName string) string {
	return entv1.Namer.Suffix(entName)
}

func ConfigName(entName string) string {
	return entv1.Namer.Suffix(entName, configSuffix)
}
