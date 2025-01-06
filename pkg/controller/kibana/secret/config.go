package secret

import (
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
)

func ConfigSecretName(kb kbv1.Kibana) string {
	return kb.Name + "-kb-config"
}
