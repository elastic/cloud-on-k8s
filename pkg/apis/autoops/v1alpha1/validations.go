package v1alpha1

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

func checkNoUnknownFields(policy *AutoOpsAgentPolicy) field.ErrorList {
	return commonv1.NoUnknownFields(policy, policy.ObjectMeta)
}

func checkNameLength(policy *AutoOpsAgentPolicy) field.ErrorList {
	return commonv1.CheckNameLength(policy)
}

func checkSupportedVersion(policy *AutoOpsAgentPolicy) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(policy.Spec.Version, version.SupportedAutoOpsAgentVersions)
}

func checkConfigSecretName(policy *AutoOpsAgentPolicy) field.ErrorList {
	if policy.Spec.Config.SecretName == "" {
		return field.ErrorList{field.Required(field.NewPath("spec").Child("config").Child("secretName"), "Config secret name must be specified")}
	}
	return nil
}
