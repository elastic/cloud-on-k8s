package logstash

import (
	"fmt"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	corev1 "k8s.io/api/core/v1"
	"path/filepath"
	"strings"
)

func buildEnv(params Params, esAssociations []commonv1.Association) ([]corev1.EnvVar, error) {
	var envs []corev1.EnvVar
	for _, assoc := range esAssociations {
		assocConf, err := assoc.AssociationConf()
		if err != nil {
			return nil, err
		}

		credentials, err := association.ElasticsearchAuthSettings(params.Context, params.Client, assoc)
		if err != nil {
			return nil, err
		}

		esRefName := normalizedNamespacedName(getEsRefNamespacedName(assoc))
		envs = append(envs, createEnvVar(esRefName+"_ES_HOSTS", assocConf.GetURL()))
		envs = append(envs, createEnvVar(esRefName+"_ES_USERNAME", credentials.Username))
		envs = append(envs, createEnvVar(esRefName+"_ES_PASSWORD", credentials.Password))

		if assocConf.GetCACertProvided() {
			caPath := filepath.Join(certificatesDir(assoc), certificates.CAFileName)
			envs = append(envs, createEnvVar(esRefName+"_ES_SSL_CERTIFICATE_AUTHORITY", caPath))
		}
	}

	return envs, nil
}

func getEsRefNamespacedName(assoc commonv1.Association) string {
	ref := assoc.AssociationRef()
	return fmt.Sprintf("%s_%s", ref.Namespace, ref.Name)
}

func normalizedNamespacedName(nn string) string {
	return strings.ToUpper(strings.ReplaceAll(nn, "-", "_"))
}

func createEnvVar(key string, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  key,
		Value: value,
	}
}
