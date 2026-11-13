// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

const (
	// WebhookPath is the HTTP path for the StackConfigPolicy validating webhook.
	WebhookPath                  = "/validate-scp-k8s-elastic-co-v1alpha1-stackconfigpolicies"
	SpecSecureSettingsDeprecated = "spec.SecureSettings is deprecated, secure settings must be set per application"
)

var (
	groupKind = schema.GroupKind{Group: GroupVersion.Group, Kind: Kind}

	defaultChecks = []func(*StackConfigPolicy) field.ErrorList{
		checkNoUnknownFields,
		checkNameLength,
		checkNoECKManagedRoleMappingsInPolicy,
		validSettings,
	}
)

// +kubebuilder:webhook:path=/validate-scp-k8s-elastic-co-v1alpha1-stackconfigpolicies,mutating=false,failurePolicy=ignore,groups=stackconfigpolicy.k8s.elastic.co,resources=stackconfigpolicies,verbs=create;update,versions=v1alpha1,name=elastic-scp-validation-v1alpha1.k8s.elastic.co,sideEffects=None,admissionReviewVersions=v1,matchPolicy=Exact

// Validate validates the StackConfigPolicy. There's no update-specific checks, so the old parameter is ignored.
func Validate(p *StackConfigPolicy, _ *StackConfigPolicy) (admission.Warnings, error) {
	warnings := admission.Warnings(p.GetWarnings())
	var errors field.ErrorList

	for _, dc := range defaultChecks {
		if err := dc(p); err != nil {
			errors = append(errors, err...)
		}
	}

	if len(errors) > 0 {
		return warnings, apierrors.NewInvalid(groupKind, p.Name, errors)
	}
	return warnings, nil
}

func (p *StackConfigPolicy) GetWarnings() []string {
	if p == nil {
		return nil
	}
	if len(p.Spec.SecureSettings) > 0 {
		return []string{fmt.Sprintf("%s %s/%s: %s", Kind, p.Namespace, p.Name, SpecSecureSettingsDeprecated)}
	}
	return nil
}

func checkNoUnknownFields(policy *StackConfigPolicy) field.ErrorList {
	return commonv1.NoUnknownFields(policy, policy.ObjectMeta)
}

func checkNameLength(policy *StackConfigPolicy) field.ErrorList {
	return commonv1.CheckNameLength(policy)
}

// ValidateReservedRoleMappingNames rejects new role mapping names that start with eck_managed_pki_. Names already in
// old (nil on creation) are allowed, since they may predate the reservation. It runs at admission only, not in
// Validate, so existing policies that use the prefix are still applied at reconcile time.
func ValidateReservedRoleMappingNames(policy *StackConfigPolicy, old *StackConfigPolicy) error {
	if errs := checkReservedRoleMappingNames(policy, old); len(errs) > 0 {
		return apierrors.NewInvalid(groupKind, policy.Name, errs)
	}
	return nil
}

func checkReservedRoleMappingNames(policy *StackConfigPolicy, old *StackConfigPolicy) field.ErrorList {
	roleMappings := policy.Spec.Elasticsearch.SecurityRoleMappings
	if roleMappings == nil {
		return nil
	}
	var oldRoleMappings map[string]any
	if old != nil && old.Spec.Elasticsearch.SecurityRoleMappings != nil {
		oldRoleMappings = old.Spec.Elasticsearch.SecurityRoleMappings.Data
	}

	var reservedNames []string
	for name := range roleMappings.Data {
		if _, existing := oldRoleMappings[name]; existing {
			continue
		}
		if strings.HasPrefix(name, commonv1.ECKPKIRoleMappingPrefix) {
			reservedNames = append(reservedNames, name)
		}
	}
	slices.Sort(reservedNames)

	path := field.NewPath("spec").Child("elasticsearch").Child("securityRoleMappings")
	var errors field.ErrorList
	for _, name := range reservedNames {
		errors = append(errors, field.Invalid(
			path.Key(name),
			name,
			fmt.Sprintf("names beginning with %q are reserved for ECK-managed PKI role mappings", commonv1.ECKPKIRoleMappingPrefix),
		))
	}
	return errors
}

func checkNoECKManagedRoleMappingsInPolicy(policy *StackConfigPolicy) field.ErrorList {
	return CheckNoECKManagedRoleMappings(policy.Spec.Elasticsearch.SecurityRoleMappings)
}

// CheckNoECKManagedRoleMappings rejects role mappings that ECK would treat as its own (see
// commonv1.IsECKManagedPKIRoleMapping), such as a copy of a generated PKI role mapping. The Elasticsearch controller
// would remove them from the file settings, and the StackConfigPolicy controller would write them again, forever.
// Unlike reserved role mapping names, it also runs at reconcile time: legacy role mappings that only use the
// eck_managed_pki_ prefix do not have this metadata, so they are still applied.
func CheckNoECKManagedRoleMappings(roleMappings *commonv1.Config) field.ErrorList {
	if roleMappings == nil {
		return nil
	}
	path := field.NewPath("spec").Child("elasticsearch").Child("securityRoleMappings")
	var errs field.ErrorList
	for _, name := range slices.Sorted(maps.Keys(roleMappings.Data)) {
		if !commonv1.IsECKManagedPKIRoleMapping(name, roleMappings.Data[name]) {
			continue
		}
		errs = append(errs, field.Invalid(
			path.Key(name),
			name,
			fmt.Sprintf("role mappings named with the %q prefix and with metadata %s: %s and %s are reserved for ECK-generated PKI role mappings, remove this metadata",
				commonv1.ECKPKIRoleMappingPrefix, commonv1.ECKPKIRoleMappingManagedByKey, commonv1.ECKPKIRoleMappingManagedByECK, commonv1.ECKPKIRoleMappingResourceUIDKey),
		))
	}
	return errs
}

// ValidateReservedPKIRealm rejects config using the reserved eck_managed_pki realm name, in any realm type, or its
// reserved order. Settings already in old (nil on creation) are allowed, since they may predate the reservation. It runs
// at admission only, not in Validate, because the policy does not know if its target clusters enable PKI. At reconcile
// time, CheckReservedPKIRealmInConfig is run only for target clusters with PKI enabled, so policies that use these
// settings are still applied to clusters that do not use PKI.
func ValidateReservedPKIRealm(policy *StackConfigPolicy, old *StackConfigPolicy) error {
	errs := CheckReservedPKIRealmInConfig(policy.Spec.Elasticsearch.Config)
	if len(errs) > 0 && old != nil {
		existing := make(map[string]struct{})
		for _, err := range CheckReservedPKIRealmInConfig(old.Spec.Elasticsearch.Config) {
			existing[err.Field] = struct{}{}
		}
		errs = slices.DeleteFunc(errs, func(err *field.Error) bool {
			_, ok := existing[err.Field]
			return ok
		})
	}
	if len(errs) > 0 {
		return apierrors.NewInvalid(groupKind, policy.Name, errs)
	}
	return nil
}

// CheckReservedPKIRealmInConfig rejects Elasticsearch config using the reserved eck_managed_pki realm name, in any realm
// type, or its reserved order. A colliding role mapping is only withheld (see withholdNameCollisions), but realm settings
// would change ECK's realm, and a duplicate realm name or order would stop Elasticsearch from starting.
func CheckReservedPKIRealmInConfig(config *commonv1.Config) field.ErrorList {
	if config == nil {
		return nil
	}
	configPath := field.NewPath("spec").Child("elasticsearch").Child("config")
	cfg, err := settings.NewCanonicalConfigFrom(config.Data)
	if err != nil {
		return field.ErrorList{field.Invalid(configPath, config, "Configuration invalid")}
	}
	return commonv1.CheckReservedPKIRealmConfig(configPath, cfg)
}

func validSettings(policy *StackConfigPolicy) field.ErrorList {
	settingsCount := 0
	if policy.Spec.Elasticsearch.ClusterSettings != nil {
		settingsCount += len(policy.Spec.Elasticsearch.ClusterSettings.Data)
	}
	if policy.Spec.Elasticsearch.SnapshotRepositories != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SnapshotRepositories.Data)
	}
	if policy.Spec.Elasticsearch.SnapshotLifecyclePolicies != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SnapshotLifecyclePolicies.Data)
	}
	if policy.Spec.Elasticsearch.SecurityRoleMappings != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SecurityRoleMappings.Data)
	}
	if policy.Spec.Elasticsearch.SecurityRoles != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SecurityRoles.Data)
	}
	if policy.Spec.Elasticsearch.IndexLifecyclePolicies != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IndexLifecyclePolicies.Data)
	}
	if policy.Spec.Elasticsearch.IngestPipelines != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IngestPipelines.Data)
	}
	if policy.Spec.Elasticsearch.IndexTemplates.ComponentTemplates != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IndexTemplates.ComponentTemplates.Data)
	}
	if policy.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates != nil {
		settingsCount += len(policy.Spec.Elasticsearch.IndexTemplates.ComposableIndexTemplates.Data)
	}
	if policy.Spec.Elasticsearch.Config != nil {
		settingsCount += len(policy.Spec.Elasticsearch.Config.Data)
	}
	if policy.Spec.Elasticsearch.SecretMounts != nil {
		settingsCount += len(policy.Spec.Elasticsearch.SecretMounts)
	}
	// Check if mountpaths in the SecretMounts are unique
	if !uniqueSecretMountPaths(policy.Spec.Elasticsearch.SecretMounts) {
		return field.ErrorList{field.Invalid(field.NewPath("spec").Child("elasticsearch").Child("secretMounts"), policy.Spec.Elasticsearch.SecretMounts, "SecretMounts cannot have duplicate mount paths")}
	}
	if policy.Spec.Kibana.Config != nil {
		settingsCount += len(policy.Spec.Kibana.Config.Data)
	}
	if settingsCount == 0 {
		return field.ErrorList{field.Required(field.NewPath("spec").Child("elasticsearch"), "One out of Elasticsearch or Kibana settings is mandatory, both must not be empty")}
	}
	return nil
}

// uniqueSecretMountPaths returns true if all given mountpaths are unique
func uniqueSecretMountPaths(secretMounts []SecretMount) bool {
	mountPathMap := make(map[string]bool)

	for _, secretMount := range secretMounts {
		if _, ok := mountPathMap[secretMount.MountPath]; ok {
			return false
		}
		mountPathMap[secretMount.MountPath] = true
	}

	return true
}
