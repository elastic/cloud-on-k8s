// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"strconv"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func AddApmKibana(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedShortName:    "apm",
		AssociationObjTemplate: func() commonv1.Association { return &apmv1.ApmKibanaAssociation{} },
		ExternalServiceURL:     getKibanaExternalURL,
		ElasticsearchRef:       getElasticsearchFromKibana,
		AssociatedNamer:        kibana.Namer,
		AssociationName:        "apm-kibana",
		AssociationLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				ApmAssociationLabelName:      associated.Name,
				ApmAssociationLabelNamespace: associated.Namespace,
				ApmAssociationLabelType:      commonv1.KibanaAssociationType,
			}
		},
		UserSecretSuffix:  "apm-kb-user",
		CASecretLabelName: kibana.KibanaNameLabelName,
		ESUserRole: func(_ commonv1.Associated) (string, error) {
			return user.ApmAgentUserRole, nil
		},
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

func getKibanaExternalURL(c k8s.Client, association commonv1.Association) (string, error) {
	kibanaRef := association.AssociationRef()
	if !kibanaRef.IsDefined() {
		return "", nil
	}
	kb := kbv1.Kibana{}
	if err := c.Get(kibanaRef.NamespacedName(), &kb); err != nil {
		return "", err
	}
	return stringsutil.Concat(kb.Spec.HTTP.Protocol(), "://", kibana.HTTPService(kb.Name), ".", kb.Namespace, ".svc:", strconv.Itoa(kibana.HTTPPort)), nil
}

// getElasticsearchFromKibana returns the Elasticsearch reference in which the user must be created for this association.
func getElasticsearchFromKibana(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
	kibanaRef := association.AssociationRef()
	if !kibanaRef.IsDefined() {
		return false, commonv1.ObjectSelector{}, nil
	}

	kb := kbv1.Kibana{}
	err := c.Get(kibanaRef.NamespacedName(), &kb)
	if errors.IsNotFound(err) {
		return false, commonv1.ObjectSelector{}, nil
	}
	if err != nil {
		return false, commonv1.ObjectSelector{}, err
	}

	return true, kb.AssociationRef(), nil
}
