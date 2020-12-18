// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package controller

import (
	"fmt"
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
		AssociatedShortName:       "apm",
		AssociatedObjTemplate:     func() commonv1.Associated { return &apmv1.ApmServer{} },
		ExternalServiceURL:        getKibanaExternalURL,
		ReferencedResourceVersion: referencedKibanaStatusVersion,
		ElasticsearchRef:          getElasticsearchFromKibana,
		AssociatedNamer:           kibana.Namer,
		AssociationName:           "apm-kibana",
		AssociationType:           commonv1.KibanaAssociationType,
		AssociatedLabels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				ApmAssociationLabelName:      associated.Name,
				ApmAssociationLabelNamespace: associated.Namespace,
				ApmAssociationLabelType:      string(commonv1.KibanaAssociationType),
			}
		},
		UserSecretSuffix: "apm-kb-user",
		ESUserRole: func(_ commonv1.Associated) (string, error) {
			return user.ApmAgentUserRole, nil
		},
		SetDynamicWatches: func(association commonv1.Association, w watches.DynamicWatches) error {
			kibanaKey := association.AssociationRef().NamespacedName()
			watchName := fmt.Sprintf("%s-%s-kibana-watch-%d", association.GetNamespace(), association.GetName(), association.Id())
			if err := w.Kibanas.AddHandler(watches.NamedWatch{
				Name:    watchName,
				Watched: []types.NamespacedName{kibanaKey},
				Watcher: k8s.ExtractNamespacedName(association),
			}); err != nil {
				return err
			}
			return nil
		},
		ClearDynamicWatches: func(associated commonv1.Associated, w watches.DynamicWatches) {
			//lookup := make(map[string]struct{})
			//for _, r := range w.Kibanas.Registrations() {
			//	lookup[r] = struct{}{}
			//}
			//
			//for _, association := range associated.GetAssociations() {
			//	watchName := fmt.Sprintf("%s-%s-kibana-watch-%d", associated.GetNamespace(), associated.GetName(), association.Id())
			//	if _, ok := lookup[watchName]; !ok {
			//		w.Kibanas.RemoveHandlerForKey(watchName)
			//	}
			//}
		},
		AssociationResourceNameLabelName:      kibana.KibanaNameLabelName,
		AssociationResourceNamespaceLabelName: kibana.KibanaNamespaceLabelName,
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

// referencedKibanaStatusVersion returns the currently running version of Kibana
// reported in its status.
func referencedKibanaStatusVersion(c k8s.Client, kbRef types.NamespacedName) (string, error) {
	var kb kbv1.Kibana
	if err := c.Get(kbRef, &kb); err != nil {
		return "", err
	}
	return kb.Status.Version, nil
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
