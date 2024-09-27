// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	ver "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana"
	kblabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/rbac"
)

// kibanaConfig is used to get the base path from the Kibana configuration.
type kibanaConfig struct {
	Server struct {
		BasePath string `yaml:"basePath"`
	}
}

func AddApmKibana(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	return association.AddAssociationController(mgr, accessReviewer, params, association.AssociationInfo{
		AssociatedShortName:       "apm",
		AssociatedObjTemplate:     func() commonv1.Associated { return &apmv1.ApmServer{} },
		ReferencedObjTemplate:     func() client.Object { return &kbv1.Kibana{} },
		ExternalServiceURL:        getKibanaExternalURL,
		ReferencedResourceVersion: referencedKibanaStatusVersion,
		ReferencedResourceNamer:   kbv1.KBNamer,
		AssociationName:           "apm-kibana",
		AssociationType:           commonv1.KibanaAssociationType,
		Labels: func(associated types.NamespacedName) map[string]string {
			return map[string]string{
				ApmAssociationLabelName:      associated.Name,
				ApmAssociationLabelNamespace: associated.Namespace,
				ApmAssociationLabelType:      commonv1.KibanaAssociationType,
			}
		},
		AssociationConfAnnotationNameBase:     commonv1.KibanaConfigAnnotationNameBase,
		AssociationResourceNameLabelName:      kblabel.KibanaNameLabelName,
		AssociationResourceNamespaceLabelName: kblabel.KibanaNamespaceLabelName,

		ElasticsearchUserCreation: &association.ElasticsearchUserCreation{
			ElasticsearchRef: getElasticsearchFromKibana,
			UserSecretSuffix: "apm-kb-user",
			ESUserRole: func(_ commonv1.Associated) (string, error) {
				return user.ApmAgentUserRole, nil
			},
		},
	})
}

func getKibanaExternalURL(c k8s.Client, assoc commonv1.Association) (string, error) {
	kibanaRef := assoc.AssociationRef()
	if !kibanaRef.IsDefined() {
		return "", nil
	}
	kb := kbv1.Kibana{}
	if err := c.Get(context.Background(), kibanaRef.NamespacedName(), &kb); err != nil {
		return "", err
	}
	serviceName := kibanaRef.ServiceName
	if serviceName == "" {
		serviceName = kbv1.HTTPService(kb.Name)
	}
	nsn := types.NamespacedName{Namespace: kb.Namespace, Name: serviceName}

	// Get Kibana base path if configured
	basePath, err := getKibanaBasePath(c, kb)
	if err != nil {
		return "", err
	}
	return association.ServiceURL(c, nsn, kb.Spec.HTTP.Protocol(), basePath)
}

func getKibanaBasePath(client k8s.Client, kb kbv1.Kibana) (string, error) {

	// base path set as ENV variable takes precedence over the one set in the Kibana config secret
	kbBasePath, err := getKibanaBasePathFromEnv(client, kb)
	if err != nil {
		return "", fmt.Errorf("failed to get Kibana base path from environment: %w", err)
	}

	if kbBasePath != "" {
		return kbBasePath, nil
	}

	if kb.Spec.Config == nil {
		return "", nil
	}

	// Get Kibana config secret to extract the basepath.
	// We are not using the Kibana CRD here for the basepath to optimize for the case where the desired and current state may differ, so we're choosing the current state to minimize any transient errors.
	kbSecretName := kibana.SecretName(kb)
	var kbConfigsecret corev1.Secret
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: kb.Namespace, Name: kbSecretName}, &kbConfigsecret); err != nil {
		return "", fmt.Errorf("failed to get Kibana base path, error getting Kibana config secret: %w", err)
	}

	kbCfg := kibanaConfig{}
	if err := yaml.Unmarshal(kbConfigsecret.Data[kibana.SettingsFilename], &kbCfg); err != nil {
		return "", fmt.Errorf("failed to get Kibana base path, unable to unmarshal Kibana config: %w", err)
	}

	if kbCfg.Server.BasePath != "" {
		return kbCfg.Server.BasePath, nil
	}

	return "", nil
}

func getKibanaBasePathFromEnv(client k8s.Client, kb kbv1.Kibana) (string, error) {
	// Get Kibana deployment to extract the basepath from the environment.
	kbDeployment := appsv1.Deployment{}
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: kb.Namespace, Name: kbv1.KBNamer.Suffix(kb.Name)}, &kbDeployment); err != nil {
		return "", fmt.Errorf("failed to get kibana deployment, error: %w", err)
	}

	return kibana.GetKibanaBasePathFromSpecEnv(kbDeployment.Spec.Template.Spec), nil
}

type kibanaVersionResponse struct {
	Version struct {
		Number      string `json:"number"`
		BuildFlavor string `json:"build_flavor"`
	} `json:"version"`
}

func (kvr kibanaVersionResponse) IsServerless() bool {
	return kvr.Version.BuildFlavor == serverlessBuildFlavor
}

func (kvr kibanaVersionResponse) GetVersion() (string, error) {
	if _, err := ver.Parse(kvr.Version.Number); err != nil {
		return "", err
	}
	return kvr.Version.Number, nil
}

// referencedKibanaStatusVersion returns the currently running version of Kibana
// reported in its status.
func referencedKibanaStatusVersion(c k8s.Client, kbAssociation commonv1.Association) (string, bool, error) {
	kbRef := kbAssociation.AssociationRef()
	if kbRef.IsExternal() {
		kbVersionResponse := &kibanaVersionResponse{}
		info, err := association.GetUnmanagedAssociationConnectionInfoFromSecret(c, kbAssociation)
		if err != nil {
			return "", false, err
		}
		ver, isServerless, err := info.Version("/api/status", kbVersionResponse)
		if err != nil {
			return "", false, err
		}
		return ver, isServerless, nil
	}

	var kb kbv1.Kibana
	err := c.Get(context.Background(), kbRef.NamespacedName(), &kb)
	if err != nil {
		return "", false, err
	}
	return kb.Status.Version, false, nil
}

// getElasticsearchFromKibana returns the Elasticsearch reference in which the user must be created for this association.
func getElasticsearchFromKibana(c k8s.Client, association commonv1.Association) (bool, commonv1.ObjectSelector, error) {
	kibanaRef := association.AssociationRef()
	if !kibanaRef.IsDefined() {
		return false, commonv1.ObjectSelector{}, nil
	}

	kb := kbv1.Kibana{}
	err := c.Get(context.Background(), kibanaRef.NamespacedName(), &kb)
	if errors.IsNotFound(err) {
		return false, commonv1.ObjectSelector{}, nil
	}
	if err != nil {
		return false, commonv1.ObjectSelector{}, err
	}

	return true, kb.EsAssociation().AssociationRef(), nil
}
