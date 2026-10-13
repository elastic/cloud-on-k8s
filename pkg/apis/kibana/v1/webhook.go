// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/stackmon/validations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
)

const (
	// WebhookPath is the HTTP path for the Kibana validating webhook.
	WebhookPath   = "/validate-kibana-k8s-elastic-co-v1-kibana"
	cfgInvalidMsg = "Configuration invalid"
)

// +kubebuilder:webhook:path=/validate-kibana-k8s-elastic-co-v1-kibana,mutating=false,failurePolicy=ignore,groups=kibana.k8s.elastic.co,resources=kibanas,verbs=create;update,versions=v1,name=elastic-kb-validation-v1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

var (
	groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

	defaultChecks = []func(*Kibana) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkMonitoring,
		checkAssociations,
		checkBackgroundTasksVersion,
		checkBackgroundTasksNodeRoles,
		checkBackgroundTasksBasePath,
		commonv1.PauseOrchestrationAnnotationCheck[*Kibana](),
	}

	updateChecks = []func(old, curr *Kibana) field.ErrorList{
		checkNoDowngrade,
	}
)

// Validate validates a Kibana resource. old is nil on create.
func Validate(k *Kibana, old *Kibana) (admission.Warnings, error) {
	return k.validate(old)
}

func (k *Kibana) validate(old *Kibana) (admission.Warnings, error) {
	var (
		errors   field.ErrorList
		warnings admission.Warnings
	)

	deprecatedWarnings, deprecatedErrors := checkIfVersionDeprecated(k)
	if len(deprecatedErrors) > 0 {
		errors = append(errors, deprecatedErrors...)
	}
	if len(deprecatedWarnings) > 0 {
		warnings = append(warnings, deprecatedWarnings)
	}
	if resourcesWarning := commonv1.PodTemplateResourcesOverrideWarning(
		"spec.resources",
		"spec.podTemplate",
		KibanaContainerName,
		k.Spec.Resources,
		k.Spec.PodTemplate,
	); resourcesWarning != "" {
		warnings = append(warnings, resourcesWarning)
	}

	if k.Spec.BackgroundTasks != nil {
		if bgResourcesWarning := commonv1.PodTemplateResourcesOverrideWarning(
			"spec.backgroundTasks.resources",
			"spec.backgroundTasks.podTemplate",
			KibanaContainerName,
			k.Spec.BackgroundTasks.Resources,
			k.Spec.BackgroundTasks.PodTemplate,
		); bgResourcesWarning != "" {
			warnings = append(warnings, bgResourcesWarning)
		}
	}

	if old != nil {
		for _, uc := range updateChecks {
			if err := uc(old, k); err != nil {
				errors = append(errors, err...)
			}
		}

		if len(errors) > 0 {
			return warnings, apierrors.NewInvalid(groupKind, k.Name, errors)
		}
	}

	for _, dc := range defaultChecks {
		if err := dc(k); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, k.Name, errors)
	}
	return warnings, nil
}

func checkNoUnknownFields(k *Kibana) field.ErrorList {
	return commonv1.NoUnknownFields(k, k.ObjectMeta)
}

func checkNameLength(k *Kibana) field.ErrorList {
	return commonv1.CheckNameLength(k)
}

func checkSupportedVersion(k *Kibana) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(k.Spec.Version, version.SupportedKibanaVersions)
}

func checkIfVersionDeprecated(k *Kibana) (string, field.ErrorList) {
	return commonv1.CheckDeprecatedStackVersion(k.Spec.Version)
}

func checkNoDowngrade(prev, curr *Kibana) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkMonitoring(k *Kibana) field.ErrorList {
	errs := validations.Validate(k, k.Spec.Version, validations.MinStackVersion)
	// Kibana must be associated to an Elasticsearch when monitoring metrics are enabled
	if monitoring.IsMetricsDefined(k) && !k.Spec.ElasticsearchRef.IsSet() {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("elasticsearchRef"), k.Spec.ElasticsearchRef,
			validations.InvalidKibanaElasticsearchRefForStackMonitoringMsg))
	}
	return errs
}

func checkAssociations(k *Kibana) field.ErrorList {
	monitoringPath := field.NewPath("spec").Child("monitoring")
	err1 := commonv1.CheckAssociationRefs(monitoringPath.Child("metrics"), k.GetMonitoringMetricsRefs()...)
	err2 := commonv1.CheckAssociationRefs(monitoringPath.Child("logs"), k.GetMonitoringLogsRefs()...)
	err3 := commonv1.CheckElasticsearchSelectorRefs(field.NewPath("spec").Child("elasticsearchRef"), k.Spec.ElasticsearchRef)
	err4 := commonv1.CheckAssociationRefs(field.NewPath("spec").Child("enterpriseSearchRef"), k.Spec.EnterpriseSearchRef)
	err5 := commonv1.CheckLocalAssociationRefs(field.NewPath("spec").Child("packageRegistryRef"), k.Spec.PackageRegistryRef)
	return append(err1, append(err2, append(err3, append(err4, err5...)...)...)...)
}

// checkBackgroundTasksVersion rejects spec.backgroundTasks on Kibana versions that predate
// BackgroundTasksMinVersion (8.16.0). The readiness probe relies on /api/status, which is
// only available without authentication from 8.16.0 onwards; background-only pods do not
// serve /login, so earlier versions would not pass the readiness check.
func checkBackgroundTasksVersion(k *Kibana) field.ErrorList {
	if k.Spec.BackgroundTasks == nil {
		return nil
	}
	v, err := version.Parse(k.Spec.Version)
	if err != nil {
		// version format already validated by checkSupportedVersion; skip here.
		return nil
	}
	if !v.GTE(BackgroundTasksMinVersion) {
		return field.ErrorList{field.Invalid(
			field.NewPath("spec").Child("backgroundTasks"),
			k.Spec.Version,
			"spec.backgroundTasks requires Kibana >= 8.16.0",
		)}
	}
	return nil
}

// checkBackgroundTasksNodeRoles rejects configurations where the user sets node.roles in
// spec.config or spec.backgroundTasks.config, or the NODE_ROLES env var on the Kibana
// container of spec.podTemplate or spec.backgroundTasks.podTemplate. ECK owns node.roles
// when background task isolation is enabled.
func checkBackgroundTasksNodeRoles(k *Kibana) field.ErrorList {
	var errs field.ErrorList
	if k.Spec.BackgroundTasks == nil {
		// No isolation: node.roles is user-controlled; no error.
		return nil
	}

	const detailsPrefix = "node.roles is managed by ECK when spec.backgroundTasks is set and must not be set"

	specPath := field.NewPath("spec")
	bgSpecPath := field.NewPath("spec").Child("backgroundTasks")

	for _, vld := range []struct {
		Config           *commonv1.Config
		PodSpec          v1.PodSpec
		Path             *field.Path
		DetailMessage    string
		EnvDetailMessage string
	}{
		{
			Config:           k.Spec.Config,
			PodSpec:          k.Spec.PodTemplate.Spec,
			Path:             specPath,
			DetailMessage:    fmt.Sprintf("%s %s", detailsPrefix, "in spec.config"),
			EnvDetailMessage: fmt.Sprintf("%s %s", detailsPrefix, "via the NODE_ROLES env var in spec.podTemplate"),
		},
		{
			Config:           k.Spec.BackgroundTasks.Config,
			PodSpec:          k.Spec.BackgroundTasks.PodTemplate.Spec,
			Path:             bgSpecPath,
			DetailMessage:    fmt.Sprintf("%s %s", detailsPrefix, "in spec.backgroundTasks.config"),
			EnvDetailMessage: fmt.Sprintf("%s %s", detailsPrefix, "via the NODE_ROLES env var in spec.backgroundTasks.podTemplate"),
		},
	} {
		if vld.Config != nil {
			config, err := common.NewCanonicalConfigFrom(vld.Config.Data)
			if err != nil {
				errs = append(errs, field.Invalid(vld.Path.Child("config"), vld.Config, cfgInvalidMsg))
			} else if keys := config.HasKeys([]string{label.NodeRolesConfigKey}); len(keys) > 0 {
				errs = append(errs, field.Forbidden(vld.Path.Child("config"), vld.DetailMessage))
			}
		}

		index, container := getKibanaContainerWithIndex(vld.PodSpec)
		if container == nil {
			continue
		}
		for _, envVar := range container.Env {
			if envVar.Name == label.NodeRolesEnvVar {
				errs = append(errs, field.Forbidden(
					vld.Path.Child("podTemplate", "spec", "containers").Index(index).Child("env"),
					vld.EnvDetailMessage,
				))
				break
			}
		}
	}

	return errs
}

// checkBackgroundTasksBasePath rejects configurations where the background tasks pool would serve
// Kibana under a different base path than the primary pool. The base path can be set through the
// server.basePath/server.rewriteBasePath configuration settings or through the SERVER_BASEPATH/
// SERVER_REWRITEBASEPATH env vars of the Kibana container; spec.backgroundTasks may repeat the
// primary values but must not change the resolved base path: both pools are reached through the
// same HTTP Service and share resources rendered from the primary base path.
func checkBackgroundTasksBasePath(k *Kibana) field.ErrorList {
	if k.Spec.BackgroundTasks == nil {
		return nil
	}

	primarySpecPath := field.NewPath("spec")
	primaryBasePathConfig, fErr := getBasePathConfig(primarySpecPath, k.Spec.PodTemplate.Spec, k.Spec.Config)
	if fErr != nil {
		return fErr
	}

	bgSpecPath := field.NewPath("spec").Child("backgroundTasks")
	bgBasePathConfig, fErr := getBasePathConfig(bgSpecPath, k.Spec.BackgroundTasks.PodTemplate.Spec, k.Spec.BackgroundTasks.Config)
	if fErr != nil {
		return fErr
	}

	if bgBasePathConfig == (basePathConfig{}) {
		return nil
	}

	if primaryBasePathConfig.GetBasePath() != bgBasePathConfig.GetBasePath() {
		return field.ErrorList{field.Forbidden(
			bgSpecPath,
			"the background tasks pool must use the same base path as the primary pool: overriding server.basePath, server.rewriteBasePath or the SERVER_BASEPATH/SERVER_REWRITEBASEPATH env vars with a different value is not supported",
		)}
	}
	return nil
}

func getBasePathConfig(fieldBasePath *field.Path, podSpec v1.PodSpec, config *commonv1.Config) (basePathConfig, field.ErrorList) {
	index, container := getKibanaContainerWithIndex(podSpec)
	cfgFromEnv, err := getKibanaBasePathFromContainerEnv(container)
	if err != nil {
		return basePathConfig{}, field.ErrorList{field.Invalid(fieldBasePath.Child("podTemplate", "spec", "containers").Index(index).Child("env"), err.Error(), cfgInvalidMsg)}
	}

	if cfgFromEnv.GetBasePath() != "" {
		return cfgFromEnv, nil
	}

	cfgFromConfig, err := getKibanaBasePathFromConfig(config)
	if err != nil {
		return basePathConfig{}, field.ErrorList{field.Invalid(fieldBasePath.Child("config"), err.Error(), cfgInvalidMsg)}
	}
	return cfgFromConfig, nil
}
