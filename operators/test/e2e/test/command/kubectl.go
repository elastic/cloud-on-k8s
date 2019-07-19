// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package command

import (
	"fmt"
)

// Kubectl represents the kubectl command.
type Kubectl struct {
	defaultArgs []string
}

func NewKubectl(kubeconfigPath string) *Kubectl {
	k := &Kubectl{}
	if kubeconfigPath != "" {
		k.defaultArgs = []string{fmt.Sprintf("--kubeconfig=%s", kubeconfigPath)}
	}

	return k
}

func (k *Kubectl) Command(command string, args ...string) *Command {
	argList := make([]string, len(k.defaultArgs)+len(args)+1)
	argList[0] = command
	copy(argList[1:], k.defaultArgs)
	copy(argList[len(k.defaultArgs)+1:], args)

	return New("kubectl", argList...).Build()
}

// Apply executes "kubectl apply"
func (k *Kubectl) Apply(args ...string) *Command {
	return k.Command("apply", args...)
}

// Get executes "kubectl get"
func (k *Kubectl) Get(args ...string) *Command {
	return k.Command("get", args...)
}
