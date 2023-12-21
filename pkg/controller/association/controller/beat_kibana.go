// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"fmt"
	"strings"

	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esuser "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

func AddBeatKibana(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedObjTemplate:     func() commonv1.Associated { return &beatv1beta1.Beat{} },
		ReferencedObjTemplate:     func() client.Object { return &kbv1.Kibana{} },
		ExternalServiceURL:        getKibanaExternalURL,
		ReferencedResourceVersion: referencedKibanaStatusVersion,
		ReferencedResourceNamer:   kbv1.KBNamer,
		AssociationName:           "beat-kibana",
		AssociatedShortName:       "beat",
		AssociationType:           commonv1.KibanaAssociationType,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				BeatAssociationLabelName:      associated.Name,
				BeatAssociationLabelNamespace: associated.Namespace,
				BeatAssociationLabelType:      commonv1.KibanaAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.KibanaConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      kblabel.KibanaNameLabelName,
		AssociationResourceNamespaceLabelName: kblabel.KibanaNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: getElasticsearchFromKibana,
			UserSecretSuffix: "beat-kb-user",
			ESUserRole:       getBeatKibanaRoles,
		},
	})
}

func getBeatKibanaRoles(associated commonv1.Associated) (string, error) {
	beat, ok := associated.(*beatv1beta1.Beat)
	if !ok {
		return "", pkgerrors.Errorf(
			"Beat expected, got %s/%s",
			associated.GetObjectKind().GroupVersionKind().Group,
			associated.GetObjectKind().GroupVersionKind().Kind,
		)
	}

	if strings.Contains(beat.Spec.Type, ",") {
		return "", fmt.Errorf("beat type %s should not contain a comma", beat.Spec.Type)
	}

	if _, ok := beatv1beta1.KnownTypes[beat.Spec.Type]; !ok {
		return fmt.Sprintf("eck_beat_kibana_%s_role", beat.Spec.Type), nil
	}

	v, err := version.Parse(beat.Spec.Version)
	if err != nil {
		return "", err
	}

	// Roles for supported Beats are based on:
	// https://www.elastic.co/guide/en/beats/filebeat/current/feature-roles.html#privileges-to-setup-beats
	// Docs are the same for all Beats. For a specific version docs change "current" to major.minor, eg:
	// https://www.elastic.co/guide/en/beats/filebeat/7.1/feature-roles.html#privileges-to-setup-beats
	switch {
	case v.GTE(version.From(7, 7, 0)):
		return strings.Join([]string{
			"kibana_admin",
			"ingest_admin",
			"beats_admin",
			esuser.BeatKibanaRoleName(esuser.V77, beat.Spec.Type),
		}, ","), nil
	case v.GTE(version.From(7, 3, 0)):
		return strings.Join([]string{
			"kibana_user",
			"ingest_admin",
			"beats_admin",
			esuser.BeatKibanaRoleName(esuser.V73, beat.Spec.Type),
		}, ","), nil
	default:
		return strings.Join([]string{
			"kibana_user",
			"ingest_admin",
			"beats_admin",
			esuser.BeatKibanaRoleName(esuser.V70, beat.Spec.Type),
		}, ","), nil
	}
}
