// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elastic/cloud-on-k8s/v2/hack/deployer/exec"
)

// Retrieving the default storage class is retried up to getDefaultScRetries times,
// waiting getDefaultScDelay between each attempt.
const (
	getDefaultScRetries = 20
	getDefaultScDelay   = 30 * time.Second
)

// createStorageClass based on default storageclass, creates new, non-default class with "volumeBindingMode: WaitForFirstConsumer"
func createStorageClass() error {
	if exists, err := exec.NewCommand("kubectl get sc").OutputContainsAny("e2e-default"); err != nil {
		return err
	} else if exists {
		return nil
	}

	log.Println("Creating storage class...")

	// When building an Autopilot GKE cluster, initially we will see the error/warning:
	// E0613 19:23:30.442396    3773 memcache.go:287] couldn't get resource list for metrics.k8s.io/v1beta1: the server is currently unable to handle the request
	// which returns exit code '0' from kubectl, which makes adjusting the storage classes fail. The following only reads stdout to try and avoid failing to read
	// the default storage class.
	defaultName, err := getDefaultStorageClassName()
	if err != nil {
		return err
	}

	sc, err := exec.NewCommand(fmt.Sprintf("kubectl get sc %s -o yaml", defaultName)).StdoutOnly().Output()
	if err != nil {
		return err
	}

	sc = strings.ReplaceAll(sc, fmt.Sprintf("name: %s", defaultName), "name: e2e-default")
	sc = strings.ReplaceAll(sc, "volumeBindingMode: Immediate", "volumeBindingMode: WaitForFirstConsumer")

	// Some providers (AKS) don't allow changing the default. To avoid having two defaults, set newly created storage
	// class to be non-default. Depending on k8s version, a different annotation is needed. To avoid parsing version
	// string, both are set.
	sc = strings.ReplaceAll(sc, `storageclass.kubernetes.io/is-default-class: "true"`, `storageclass.kubernetes.io/is-default-class: "false"`)
	sc = strings.ReplaceAll(sc, `storageclass.beta.kubernetes.io/is-default-class: "true"`, `storageclass.beta.kubernetes.io/is-default-class: "false"`)
	return exec.NewCommand(fmt.Sprintf(`cat <<EOF | kubectl apply -f -
%s
EOF`, sc)).Run()
}

func setupDisks(plan Plan) error {
	if plan.DiskSetup == "" {
		return nil
	}
	return exec.NewCommand(plan.DiskSetup).Run()
}

func getDefaultStorageClassName() (string, error) {
	get := func() (string, error) {
		for _, annotation := range []string{
			`storageclass\.kubernetes\.io/is-default-class`,
			`storageclass\.beta\.kubernetes\.io/is-default-class`,
		} {
			template := `kubectl get sc -o=jsonpath="{$.items[?(@.metadata.annotations.%s=='true')].metadata.name}"`
			baseScs, err := exec.NewCommand(fmt.Sprintf(template, annotation)).StdoutOnly().OutputList()
			if err != nil {
				return "", err
			}

			if len(baseScs) != 0 {
				return baseScs[0], nil
			}
		}
		return "", fmt.Errorf("default storageclass not found")
	}

	// Some providers (AKS) may not have the default storage class created yet,
	// let's retry getting it several times in case that happens.
	attempt := 1
	for {
		name, err := get()
		if err != nil && attempt < getDefaultScRetries {
			log.Printf("failed to retrieve the default storageclass, retrying in %s: %s\n", getDefaultScDelay, err.Error())
			time.Sleep(getDefaultScDelay)
			attempt++
			continue
		}
		return name, err
	}
}
