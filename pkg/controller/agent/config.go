// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"errors"
	"fmt"
	"hash"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type connectionSettings struct {
	host, ca, username, password string
}

func reconcileConfig(params Params, configHash hash.Hash) *reconciler.Results {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	cfgBytes, err := buildConfig(params)
	if err != nil {
		return results.WithError(err)
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: params.Agent.Namespace,
			Name:      ConfigSecretName(params.Agent.Name),
			Labels:    common.AddCredentialsLabel(NewLabels(params.Agent)),
		},
		Data: map[string][]byte{
			ConfigFileName: cfgBytes,
		},
	}

	if _, err = reconciler.ReconcileSecret(params.Client, expected, &params.Agent); err != nil {
		return results.WithError(err)
	}

	_, _ = configHash.Write(cfgBytes)

	return results
}

func buildConfig(params Params) ([]byte, error) {
	cfg, err := buildOutputConfig(params)
	if err != nil {
		return nil, err
	}

	// get user config from `config` or `configRef`
	userConfig, err := getUserConfig(params)
	if err != nil {
		return nil, err
	}

	if err = cfg.MergeWith(userConfig); err != nil {
		return nil, err
	}

	return cfg.Render()
}

func buildOutputConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Agent.Spec.FleetModeEnabled() {
		// in fleet mode outputs are owned by fleet
		return settings.NewCanonicalConfig(), nil
	}

	allAssociations := params.Agent.GetAssociations()

	var esAssociations []commonv1.Association
	for _, assoc := range allAssociations {
		if assoc.AssociationType() == commonv1.ElasticsearchAssociationType {
			esAssociations = append(esAssociations, assoc)
		}
	}

	for _, assoc := range esAssociations {
		if !assoc.AssociationConf().IsConfigured() {
			return settings.NewCanonicalConfig(), nil
		}
	}

	outputs := map[string]interface{}{}
	for i, assoc := range esAssociations {
		username, password, err := association.ElasticsearchAuthSettings(params.Client, assoc)
		if err != nil {
			return settings.NewCanonicalConfig(), err
		}

		output := map[string]interface{}{
			"type":     "elasticsearch",
			"username": username,
			"password": password,
			"hosts":    []string{assoc.AssociationConf().GetURL()},
		}
		if assoc.AssociationConf().GetCACertProvided() {
			output["ssl.certificate_authorities"] = []string{path.Join(certificatesDir(assoc), CAFileName)}
		}

		outputName := params.Agent.Spec.ElasticsearchRefs[i].OutputName
		if outputName == "" {
			if len(esAssociations) > 1 {
				return settings.NewCanonicalConfig(), errors.New("output is not named and there is more than one specified")
			}
			outputName = "default"
		}
		outputs[outputName] = output
	}

	return settings.NewCanonicalConfigFrom(map[string]interface{}{
		"outputs": outputs,
	})
}

// getUserConfig extracts the config either from the spec `config` field or from the Secret referenced by spec
// `configRef` field.
func getUserConfig(params Params) (*settings.CanonicalConfig, error) {
	if params.Agent.Spec.Config != nil {
		return settings.NewCanonicalConfigFrom(params.Agent.Spec.Config.Data)
	}
	return common.ParseConfigRef(params, &params.Agent, params.Agent.Spec.ConfigRef, ConfigFileName)
}

func extractConnectionSettings(
	agent agentv1alpha1.Agent,
	client k8s.Client,
	associationType commonv1.AssociationType,
) (connectionSettings, error) {
	assoc, err := association.SingleAssociationOfType(agent.GetAssociations(), associationType)
	if err != nil {
		return connectionSettings{}, err
	}

	if assoc == nil {
		errTemplate := "association of type %s not found in %d associations"
		return connectionSettings{}, fmt.Errorf(errTemplate, associationType, len(agent.GetAssociations()))
	}

	username, password, err := association.ElasticsearchAuthSettings(client, assoc)
	if err != nil {
		return connectionSettings{}, err
	}

	ca := ""
	if assoc.AssociationConf().GetCACertProvided() {
		ca = path.Join(certificatesDir(assoc), CAFileName)
	}

	return connectionSettings{
		host:     assoc.AssociationConf().GetURL(),
		ca:       ca,
		username: username,
		password: password,
	}, err
}
