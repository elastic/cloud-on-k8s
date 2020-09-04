// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

var (
	StorageClassProvisionerRegExp = regexp.MustCompile(`provisioner:.+\n`)
	DefaultStorageClass           = ""
	NoProvisioner                 = "kubernetes.io/no-provisioner"
)

// Retrieving the default storage class is retried up to getDefaultScRetries times,
// waiting getDefaultScDelay between each attempt.
const (
	getDefaultScRetries = 20
	getDefaultScDelay   = 30 * time.Second
)

// createStorageClass based on default storageclass, creates new, non-default class with "volumeBindingMode: WaitForFirstConsumer"
func createStorageClass(provisioner string) error {
	log.Println("Creating storage class...")

	if exists, err := NewCommand("kubectl get sc").OutputContainsAny("e2e-default"); err != nil {
		return err
	} else if exists {
		return nil
	}

	defaultName, err := getDefaultStorageClassName()
	if err != nil {
		return err
	}

	sc, err := NewCommand(fmt.Sprintf("kubectl get sc %s -o yaml", defaultName)).Output()
	if err != nil {
		return err
	}

	sc = strings.Replace(sc, fmt.Sprintf("name: %s", defaultName), "name: e2e-default", -1)
	sc = strings.Replace(sc, "volumeBindingMode: Immediate", "volumeBindingMode: WaitForFirstConsumer", -1)
	if provisioner != "" {
		sc = StorageClassProvisionerRegExp.ReplaceAllString(sc, fmt.Sprintf("provisioner: %s\n", provisioner))
	}
	// Some providers (AKS) don't allow changing the default. To avoid having two defaults, set newly created storage
	// class to be non-default. Depending on k8s version, a different annotation is needed. To avoid parsing version
	// string, both are set.
	sc = strings.Replace(sc, `storageclass.kubernetes.io/is-default-class: "true"`, `storageclass.kubernetes.io/is-default-class: "false"`, -1)
	sc = strings.Replace(sc, `storageclass.beta.kubernetes.io/is-default-class: "true"`, `storageclass.beta.kubernetes.io/is-default-class: "false"`, -1)
	return NewCommand(fmt.Sprintf(`cat <<EOF | kubectl apply -f -
%s
EOF`, sc)).Run()
}

func getDefaultStorageClassName() (string, error) {
	get := func() (string, error) {
		for _, annotation := range []string{
			`storageclass\.kubernetes\.io/is-default-class`,
			`storageclass\.beta\.kubernetes\.io/is-default-class`,
		} {
			template := `kubectl get sc -o=jsonpath="{$.items[?(@.metadata.annotations.%s=='true')].metadata.name}"`
			baseScs, err := NewCommand(fmt.Sprintf(template, annotation)).OutputList()
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
