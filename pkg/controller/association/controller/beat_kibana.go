// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func AddBeatKibana(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociationObjTemplate: func() commonv1.Association { return &beatv1beta1.BeatKibanaAssociation{} },
		ElasticsearchRef:       getElasticsearchFromKibana,
		ExternalServiceURL:     getKibanaExternalURL,
		AssociatedNamer:        kibana.Namer,
		AssociationName:        "beat-kibana",
		AssociatedShortName:    "beat",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				BeatAssociationLabelName:      associated.Name,
				BeatAssociationLabelNamespace: associated.Namespace,
				BeatAssociationLabelType:      commonv1.KibanaAssociationType,
			}
		},
		UserSecretSuffix:  "beat-kb-user",
		CASecretLabelName: kibana.KibanaNameLabelName,
		ESUserRole: func(commonv1.Associated) (string, error) {
			return user.SuperUserBuiltinRole, nil
		},
		// The generic association controller watches Elasticsearch by default but we are interested in changes to
		// Kibana as well for the purposes of establishing the association.
		SetDynamicWatches: func(association commonv1.Association, w watches.DynamicWatches) error {
			kibanaKey := association.AssociationRef().NamespacedName()
			watchName := association.GetNamespace() + "-" + association.GetName() + "-kibana-watch"
			if err := w.Kibanas.AddHandler(watches.NamedWatch{
				Name:    watchName,
				Watched: []types.NamespacedName{kibanaKey},
				Watcher: k8s.ExtractNamespacedName(association),
			}); err != nil {
				return err
			}
			return nil
		},
		ClearDynamicWatches: func(associated types.NamespacedName, w watches.DynamicWatches) {
			watchName := associated.Namespace + "-" + associated.Name + "-kibana-watch"
			w.Kibanas.RemoveHandlerForKey(watchName)
		},
	})
}
