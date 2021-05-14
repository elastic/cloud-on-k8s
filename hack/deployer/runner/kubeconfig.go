package runner

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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
	merged, err := NewCommand("kubectl config view --flatten").
		WithLog("Merging kubeconfig with").
		WithoutStreaming().
		WithVariable("KUBECONFIG", fmt.Sprintf("%s:%s", hostKubeconfig, kubeConfig)).
		Output()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(hostKubeconfig, []byte(merged), 0600)
}

func copyFile(src, tgt string) error {
	if err := os.MkdirAll(filepath.Dir(tgt), os.ModePerm); err != nil {
		return err
	}
	cmd := fmt.Sprintf("cp %s %s", src, tgt)
	return NewCommand(cmd).WithoutStreaming().WithLog("Copying kubeconfig").Run()
}
