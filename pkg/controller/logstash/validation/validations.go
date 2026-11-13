// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"context"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	volumevalidations "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume/validations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	netutil "github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

const (
	pvcImmutableMsg    = "Volume claim templates can only have storage requests modified"
	invalidSanIPErrMsg = "Invalid SAN IP address. Must be a valid IPv4 or IPv6 address"
	// apiServiceName is the name of the Service of the Logstash API in spec.services.
	apiServiceName = "api"
)

type validation func(*lsv1alpha1.Logstash) field.ErrorList

type updateValidation func(*lsv1alpha1.Logstash, *lsv1alpha1.Logstash) field.ErrorList

// updateValidations are the validation funcs that only apply to updates
func updateValidations(ctx context.Context, k8sClient k8s.Client, validateStorageClass bool) []updateValidation {
	return []updateValidation{
		checkNoDowngrade,
		func(current *lsv1alpha1.Logstash, proposed *lsv1alpha1.Logstash) field.ErrorList {
			return checkPVCchanges(ctx, current, proposed, k8sClient, validateStorageClass)
		},
	}
}

// validations are the validation funcs that apply to creates or updates
func validations() []validation {
	return []validation{
		checkNoUnknownFields,
		checkNameLength,
		checkSupportedVersion,
		checkSingleConfigSource,
		checkESRefsNamed,
		checkAssociations,
		checkConsistentESRefAuthentication,
		checkSinglePipelineSource,
		checkPipelinesRefSource,
		checkServicesSubjectAltNames,
		commonv1.PauseOrchestrationAnnotationCheck[*lsv1alpha1.Logstash](),
	}
}

func checkNoUnknownFields(l *lsv1alpha1.Logstash) field.ErrorList {
	return commonv1.NoUnknownFields(l, l.ObjectMeta)
}

func checkNameLength(l *lsv1alpha1.Logstash) field.ErrorList {
	return commonv1.CheckNameLength(l)
}

func checkSupportedVersion(l *lsv1alpha1.Logstash) field.ErrorList {
	return commonv1.CheckSupportedStackVersion(l.Spec.Version, version.SupportedLogstashVersions)
}

func checkNoDowngrade(prev, curr *lsv1alpha1.Logstash) field.ErrorList {
	if commonv1.IsConfiguredToAllowDowngrades(curr) {
		return nil
	}
	return commonv1.CheckNoDowngrade(prev.Spec.Version, curr.Spec.Version)
}

func checkSingleConfigSource(l *lsv1alpha1.Logstash) field.ErrorList {
	if l.Spec.Config != nil && l.Spec.ConfigRef != nil {
		msg := "Specify at most one of [`config`, `configRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("config"), msg),
			field.Forbidden(field.NewPath("spec").Child("configRef"), msg),
		}
	}

	return nil
}

func checkAssociations(l *lsv1alpha1.Logstash) field.ErrorList {
	monitoringPath := field.NewPath("spec").Child("monitoring")
	err1 := commonv1.CheckAssociationRefs(monitoringPath.Child("metrics"), l.GetMonitoringMetricsRefs()...)
	err2 := commonv1.CheckAssociationRefs(monitoringPath.Child("logs"), l.GetMonitoringLogsRefs()...)
	err3 := commonv1.CheckElasticsearchSelectorRefs(field.NewPath("spec").Child("elasticsearchRefs"), l.ElasticsearchRefs()...)
	return append(append(err1, err2...), err3...)
}

func checkConsistentESRefAuthentication(l *lsv1alpha1.Logstash) field.ErrorList {
	return commonv1.CheckConsistentESRefAuthentication(field.NewPath("spec").Child("elasticsearchRefs"), l.Namespace, l.ElasticsearchRefs()...)
}

func checkSinglePipelineSource(a *lsv1alpha1.Logstash) field.ErrorList {
	if a.Spec.Pipelines != nil && a.Spec.PipelinesRef != nil {
		msg := "Specify at most one of [`pipelines`, `pipelinesRef`], not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("pipelines"), msg),
			field.Forbidden(field.NewPath("spec").Child("pipelinesRef"), msg),
		}
	}

	return nil
}

func checkPipelinesRefSource(a *lsv1alpha1.Logstash) field.ErrorList {
	if a.Spec.PipelinesRef == nil {
		return nil
	}
	ref := a.Spec.PipelinesRef
	if ref.SecretName != "" && ref.ConfigMapName != "" {
		msg := "Specify at most one of [`secretName`, `configMapName`] in pipelinesRef, not both"
		return field.ErrorList{
			field.Forbidden(field.NewPath("spec").Child("pipelinesRef").Child("secretName"), msg),
			field.Forbidden(field.NewPath("spec").Child("pipelinesRef").Child("configMapName"), msg),
		}
	}
	return nil
}

// checkServicesSubjectAltNames ensures that the IP addresses set in the subject alternative names of the Services
// are valid.
func checkServicesSubjectAltNames(l *lsv1alpha1.Logstash) field.ErrorList {
	var errs field.ErrorList
	for i, service := range l.Spec.Services {
		selfSigned := service.TLS.SelfSignedCertificate
		if selfSigned == nil {
			continue
		}
		for j, san := range selfSigned.SubjectAlternativeNames {
			if san.IP != "" && netutil.IPToRFCForm(net.ParseIP(san.IP)) == nil {
				errs = append(errs, field.Invalid(
					field.NewPath("spec").Child("services").Index(i).Child("tls", "selfSignedCertificate", "subjectAltNames").Index(j).Child("ip"),
					san.IP,
					invalidSanIPErrMsg,
				))
			}
		}
	}
	return errs
}

// servicesTLSWarnings returns warnings for the TLS options of the Services which have no effect.
// Logstash uses a single certificate, issued according to the TLS options of the API Service. The other Services
// can only add subject alternative names to it, and only if this certificate is self-signed.
func servicesTLSWarnings(l *lsv1alpha1.Logstash) []string {
	var warnings []string
	apiCustomCert := l.APIServerTLSOptions().Certificate.SecretName != ""
	for i, service := range l.Spec.Services {
		if l.IsAPIService(service) {
			continue
		}
		tlsPath := field.NewPath("spec").Child("services").Index(i).Child("tls")
		if service.TLS.Certificate.SecretName != "" {
			warnings = append(warnings, fmt.Sprintf(
				"%s is ignored: Logstash uses a single certificate, which is configured in the TLS options of the `%s` Service",
				tlsPath.Child("certificate"), apiServiceName,
			))
		}
		if service.TLS.CertificateAuthorities.ConfigMapName != "" {
			warnings = append(warnings, fmt.Sprintf(
				"%s is ignored: it is only supported in the TLS options of the `%s` Service",
				tlsPath.Child("certificateAuthorities"), apiServiceName,
			))
		}
		selfSigned := service.TLS.SelfSignedCertificate
		if apiCustomCert && selfSigned != nil && !selfSigned.Disabled && len(selfSigned.SubjectAlternativeNames) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"%s is ignored: the `%s` Service uses a custom certificate, to which subject alternative names cannot be added",
				tlsPath.Child("selfSignedCertificate", "subjectAltNames"), apiServiceName,
			))
		}
	}
	return warnings
}

func checkESRefsNamed(l *lsv1alpha1.Logstash) field.ErrorList {
	var errorList field.ErrorList
	for i, esRef := range l.Spec.ElasticsearchRefs {
		if esRef.ClusterName == "" {
			errorList = append(
				errorList,
				field.Required(
					field.NewPath("spec").Child("elasticsearchRefs").Index(i).Child("clusterName"),
					fmt.Sprintf("clusterName is a mandatory field - missing on %v", esRef.NamespacedName())),
			)
		}
	}
	return errorList
}

// checkPVCchanges ensures no PVCs are changed, as volume claim templates are immutable in StatefulSets.
func checkPVCchanges(ctx context.Context, current *lsv1alpha1.Logstash, proposed *lsv1alpha1.Logstash, k8sClient k8s.Client, validateStorageClass bool) field.ErrorList {
	var errs field.ErrorList
	if current == nil || proposed == nil {
		return errs
	}

	// Check that no modification was made to the claims, except on storage requests.
	if !apiequality.Semantic.DeepEqual(
		claimsWithoutStorageReq(current.Spec.VolumeClaimTemplates),
		claimsWithoutStorageReq(proposed.Spec.VolumeClaimTemplates),
	) {
		errs = append(errs, field.Invalid(field.NewPath("spec").Child("volumeClaimTemplates"), proposed.Spec.VolumeClaimTemplates, pvcImmutableMsg))
	}

	if err := volumevalidations.ValidateClaimsStorageUpdate(ctx, k8sClient, current.Spec.VolumeClaimTemplates, proposed.Spec.VolumeClaimTemplates, validateStorageClass); err != nil {
		errs = append(errs, field.Invalid(
			field.NewPath("spec").Child("volumeClaimTemplates"),
			proposed.Spec.VolumeClaimTemplates,
			err.Error(),
		))
	}

	return errs
}

// claimsWithoutStorageReq returns a copy of the given claims, with all storage requests set to the empty quantity.
func claimsWithoutStorageReq(claims []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	result := make([]corev1.PersistentVolumeClaim, 0, len(claims))
	for _, claim := range claims {
		patchedClaim := *claim.DeepCopy()
		patchedClaim.Spec.Resources.Requests[corev1.ResourceStorage] = resource.Quantity{}
		result = append(result, patchedClaim)
	}
	return result
}

func check(ls *lsv1alpha1.Logstash, validations []validation) field.ErrorList {
	var errs field.ErrorList
	for _, val := range validations {
		if err := val(ls); err != nil {
			errs = append(errs, err...)
		}
	}
	return errs
}
