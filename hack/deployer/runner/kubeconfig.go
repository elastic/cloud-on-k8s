// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
)

func mergeKubeconfig(kubeConfig string) error {
	// 1. do we have something to copy?
	if _, err := os.Stat(kubeConfig); os.IsNotExist(err) {
		return errors.New(" kubeconfig file does not exist")
	}
	// 2. is there any existing kubeconfig?
	hostKubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	if _, err := os.Stat(hostKubeconfig); os.IsNotExist(err) {
		// if no just copy it over
		return copyFile(kubeConfig, hostKubeconfig)
	}
	// 3. merge: load both configs and upsert new entries into existing ones.
	// New entries take precedence for same-named clusters/users/contexts (e.g. a
	// recreated kind cluster now listening on a different port).
	log.Printf("Merging kubeconfig %s into %s", kubeConfig, hostKubeconfig)
	newCfg, err := clientcmd.LoadFromFile(kubeConfig)
	if err != nil {
		return err
	}
	existingCfg, err := clientcmd.LoadFromFile(hostKubeconfig)
	if err != nil {
		return err
	}
	for k, v := range newCfg.Clusters {
		existingCfg.Clusters[k] = v
	}
	for k, v := range newCfg.AuthInfos {
		existingCfg.AuthInfos[k] = v
	}
	for k, v := range newCfg.Contexts {
		existingCfg.Contexts[k] = v
	}
	existingCfg.CurrentContext = newCfg.CurrentContext

	// 4. write back atomically via a temp file + rename to avoid partial-write corruption.
	data, err := clientcmd.Write(*existingCfg)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(hostKubeconfig), "kubeconfig-tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), hostKubeconfig)
}

func removeKubeconfig(context, clusterName, userName string) error {
	params := map[string]any{
		"Context": context,
	}
	if err := exec.NewCommand("kubectl config get-contexts {{.Context}}").
		AsTemplate(params).Run(); err != nil {
		// skip because the admin context does not exist in the kube config
		return nil //nolint:nilerr
	}

	log.Printf("Removing context, user and cluster entry from kube config")
	if err := exec.NewCommand("kubectl config delete-context {{.Context}}").
		AsTemplate(params).Run(); err != nil {
		return err
	}
	if err := exec.NewCommand("kubectl config unset users.{{.User}}").
		AsTemplate(map[string]any{
			"User": userName,
		}).Run(); err != nil {
		return err
	}
	return exec.NewCommand("kubectl config delete-cluster {{.ClusterName}}").
		AsTemplate(map[string]any{"ClusterName": clusterName}).
		Run()
}

func copyFile(src, tgt string) error {
	if err := os.MkdirAll(filepath.Dir(tgt), os.ModePerm); err != nil {
		return err
	}
	cmd := fmt.Sprintf("cp %s %s", src, tgt)
	return exec.NewCommand(cmd).WithoutStreaming().WithLog("Copying kubeconfig").Run()
}

// activateKubeconfig activates the kubeconfig for the given cluster.
// Intended to be run after merging of the kubeconfig has already happened.
// Currently only used in the k3d runner, as it doesn't properly handle kubeconfig operations.
func activateKubeconfig(clusterName string) error {
	return exec.NewCommand("kubectl config use-context " + clusterName).
		Run()
}
