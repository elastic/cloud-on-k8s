package pvgc

import (
	"flag"
	"fmt"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"os/user"
	"path/filepath"
)

var (
	kubeconfig, masterURL string
)

func init() {
	// TODO: Fix this to allow double vendoring this library but still register flags on behalf of users
	flag.StringVar(&kubeconfig, "kubeconfig", "",
		"Paths to a kubeconfig. Only required if out-of-cluster.")

	flag.StringVar(&masterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. "+
			"Only required if out-of-cluster.")
}

// GetConfig creates a *rest.Config for talking to a Kubernetes apiserver.
// If --kubeconfig is set, will use the kubeconfig file at that location.  Otherwise will assume running
// in cluster and use the cluster provided kubeconfig.
//
// Config precedence
//
// * --kubeconfig flag pointing at a file
//
// * KUBECONFIG environment variable pointing at a file
//
// * In-cluster config if running in cluster
//
// * $HOME/.kube/config if exists
func GetConfig() (*rest.Config, error) {
	// If a flag is specified with the config location, use that
	if len(kubeconfig) > 0 {
		return clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	}
	// If an env variable is specified with the config locaiton, use that
	if len(os.Getenv("KUBECONFIG")) > 0 {
		return clientcmd.BuildConfigFromFlags(masterURL, os.Getenv("KUBECONFIG"))
	}
	// If no explicit location, try the in-cluster config
	if c, err := rest.InClusterConfig(); err == nil {
		return c, nil
	}
	// If no in-cluster config, try the default location in the user's home directory
	if usr, err := user.Current(); err == nil {
		if c, err := clientcmd.BuildConfigFromFlags(
			"", filepath.Join(usr.HomeDir, ".kube", "config")); err == nil {
			return c, nil
		}
	}

	return nil, fmt.Errorf("could not locate a kubeconfig")
}
