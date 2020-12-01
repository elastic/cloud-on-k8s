// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"errors"
	"hash"
	"path"

	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func reconcileConfig(params Params, configHash hash.Hash) error {
	cfgBytes, err := buildConfig(params)
	if err != nil {
		return err
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
		return err
	}

	_, _ = configHash.Write(cfgBytes)

	return nil
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

	if userConfig == nil {
		return cfg.Render()
	}

	if err = cfg.MergeWith(userConfig); err != nil {
		return nil, err
	}

	return cfg.Render()
}

func buildOutputConfig(params Params) (*settings.CanonicalConfig, error) {
	associations := params.Agent.GetAssociations()

	for _, assoc := range associations {
		if !assoc.AssociationConf().IsConfigured() {
			return settings.NewCanonicalConfig(), nil
		}
	}

	outputs := map[string]interface{}{}
	for _, assoc := range associations {
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

		//a := (assoc).(*v1alpha1.AgentESAssociation)
		outputName := "" // todo agent a.Output.OutputName

		if outputName == "" {
			if len(associations) > 1 {
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
