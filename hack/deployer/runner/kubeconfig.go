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

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
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
	// 3. if there is existing configuration  attempt to merge both
	merged, err := exec.NewCommand("kubectl config view --flatten").
		WithLog("Merging kubeconfig with").
		WithoutStreaming().
		WithVariable("KUBECONFIG", fmt.Sprintf("%s:%s", hostKubeconfig, kubeConfig)).
		Output()
	if err != nil {
		return err
	}
	return os.WriteFile(hostKubeconfig, []byte(merged), 0600)
}

func removeKubeconfig(context, clusterName, userName string) error {
	params := map[string]interface{}{
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
		AsTemplate(map[string]interface{}{
			"User": userName,
		}).Run(); err != nil {
		return err
	}
	return exec.NewCommand("kubectl config delete-cluster {{.ClusterName}}").
		AsTemplate(map[string]interface{}{"ClusterName": clusterName}).
		Run()
}

func copyFile(src, tgt string) error {
	if err := os.MkdirAll(filepath.Dir(tgt), os.ModePerm); err != nil {
		return err
	}
	cmd := fmt.Sprintf("cp %s %s", src, tgt)
	return exec.NewCommand(cmd).WithoutStreaming().WithLog("Copying kubeconfig").Run()
}
