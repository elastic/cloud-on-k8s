// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package runner

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"text/template"
)

const (
	OcpDriverID                     = "ocp"
	OcpVaultPath                    = "secret/devops-ci/cloud-on-k8s/ci-ocp-k8s-operator"
	OcpServiceAccountVaultFieldName = "service-account"
	OcpPullSecretFieldName          = "ocp-pull-secret" // nolint:gosec
	OcpStateBucket                  = "eck-deployer-ocp-clusters-state"
	OcpConfigFileName               = "deployer-config-ocp.yml"
	DefaultOcpRunConfigTemplate     = `id: ocp-dev
overrides:
  clusterName: %s-dev-cluster
  ocp:
    gCloudProject: %s
    pullSecret: '%s'
`

	OcpInstallerConfigTemplate = `apiVersion: v1
baseDomain: {{.BaseDomain}}
compute:
- hyperthreading: Enabled
  name: worker
  platform:
    gcp:
      type: {{.MachineType}}
  replicas: {{.NodeCount}}
controlPlane:
  hyperthreading: Enabled
  name: master
  platform:
    gcp:
      type: {{.MachineType}}
  replicas: {{.NodeCount}}
metadata:
  creationTimestamp: null
  name: {{.ClusterName}}
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineCIDR: 10.0.0.0/16
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
platform:
  gcp:
    projectID: {{.GCloudProject}}
    region: {{.Region}}
pullSecret: '{{.PullSecret}}'`
)

func init() {
	drivers[OcpDriverID] = &OcpDriverFactory{}
}

type OcpDriverFactory struct {
}

type OcpDriver struct {
	plan Plan
	ctx  map[string]interface{}
}

func (gdf *OcpDriverFactory) Create(plan Plan) (Driver, error) {
	baseDomain := plan.Ocp.BaseDomain

	// Domains used for the OCP deployment must be
	// pre-configured on the destination cloud. A zone
	// for these domains must exist and it has to be
	// reachable from the internet as `openshift-installer`
	// interacts with the deployed OCP cluster to monitor
	// and complete the deployment.
	//
	// The default `eck-ocp.elastic.dev` subdomain is configured
	// on AWS as an NS record and points to a zone configured in
	// the `elastic-cloud-dev` project on GCP.
	if baseDomain == "" {
		baseDomain = "eck-ocp.elastic.dev"
	}
	return &OcpDriver{
		plan: plan,
		ctx: map[string]interface{}{
			"GCloudProject":              plan.Ocp.GCloudProject,
			"ClusterName":                plan.ClusterName,
			"Region":                     plan.Ocp.Region,
			"AdminUsername":              plan.Ocp.AdminUsername,
			"KubernetesVersion":          plan.KubernetesVersion,
			"MachineType":                plan.MachineType,
			"LocalSsdCount":              plan.Ocp.LocalSsdCount,
			"NodeCount":                  plan.Ocp.NodeCount,
			"BaseDomain":                 baseDomain,
			"WorkDir":                    plan.Ocp.WorkDir,
			"OcpStateBucket":             OcpStateBucket,
			"PullSecret":                 plan.Ocp.PullSecret,
			"OverwriteDefaultKubeconfig": plan.Ocp.OverwriteDefaultKubeconfig,
			"Authenticated":              false,
		},
	}, nil
}

func (d *OcpDriver) Execute() error {
	cleanUp, err := d.ensureContext()
	defer cleanUp()
	if err != nil {
		return err
	}

	if d.ctx["PullSecret"] == nil || d.ctx["PullSecret"] == "" {
		client, err := NewClient(*d.plan.VaultInfo)
		if err != nil {
			return err
		}

		d.ctx["PullSecret"], _ = client.Get(OcpVaultPath, "pull-secret")
	}

	status := d.currentStatus()

	switch d.plan.Operation {
	case DeleteAction:
		if status != NotFound {
			// always attempt a deletion
			err = d.delete()
		} else {
			log.Printf("Not deleting as cluster doesn't exist")
		}
	case CreateAction:
		if status == Running {
			log.Printf("Not creating as cluster exists")

			if err := d.uploadCredentials(); err != nil {
				return err
			}

		} else if err := d.create(); err != nil {
			return err
		}

		if err := d.GetCredentials(); err != nil {
			return err
		}

		if err := setupDisks(d.plan); err != nil {
			return err
		}
		if err := createStorageClass(); err != nil {
			return err
		}
	default:
		err = fmt.Errorf("unknown operation %s", d.plan.Operation)
	}

	return err
}

func (d *OcpDriver) ensureContext() (cleanUp func(), err error) {
	cleanUp = func() {} // NOOP
	if d.ctx["WorkDir"].(string) == "" {
		dir, err := ioutil.TempDir("", d.ctx["ClusterName"].(string))
		if err != nil {
			log.Fatal(err)
		}

		cleanUp = func() {
			os.RemoveAll(dir)
		}
		d.ctx["WorkDir"] = dir
		log.Printf("Created WorkDir: %s", d.ctx["WorkDir"])
	}

	if _, exists := d.ctx["ClusterStateDir"]; !exists {
		stateDir := filepath.Join(d.ctx["WorkDir"].(string), d.ctx["ClusterName"].(string))
		if err := os.MkdirAll(stateDir, os.ModePerm); err != nil {
			return cleanUp, err
		}
		d.ctx["ClusterStateDir"] = stateDir
		log.Printf("Using ClusterStateDir: %s", stateDir)
	}

	// avoid double authentication
	if d.ctx["Authenticated"].(bool) {
		return cleanUp, nil
	}

	if err := authToGCP(
		d.plan.VaultInfo, OcpVaultPath, OcpServiceAccountVaultFieldName,
		d.plan.ServiceAccount, false, d.ctx["GCloudProject"],
	); err != nil {
		return cleanUp, err
	}
	d.ctx["Authenticated"] = true
	return cleanUp, nil
}

type ClusterStatus string

var (
	NotFound      ClusterStatus = "NotFound"
	NotResponding ClusterStatus = "NotResponding"
	Running       ClusterStatus = "Running"
)

func (d *OcpDriver) currentStatus() ClusterStatus {
	log.Println("Checking if cluster exists...")

	err := d.GetCredentials()

	if err != nil {
		// No need to send this error back
		// in this case. We're checking whether
		// the cluster exists and an error
		// getting the credentials is expected for non
		// existing clusters.
		return NotFound
	}

	log.Println("Cluster state synced: Testing that the OpenShift cluster is alive... ")
	kubeConfig := filepath.Join(d.ctx["WorkDir"].(string), d.ctx["ClusterName"].(string), "auth", "kubeconfig")
	cmd := "kubectl version"
	alive, err := NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().WithVariable("KUBECONFIG", kubeConfig).OutputContainsAny("Server Version")

	if !alive || err != nil { // error will be typically not nil when alive is false but let's be explicit here to avoid returning Running on a non-nil error
		log.Printf("a cluster state dir was found in %s but the cluster is not responding to `kubectl version`: %s", d.ctx["ClusterStateDir"], err.Error())
		return NotResponding
	}

	return Running
}

func (d *OcpDriver) create() error {
	log.Println("Creating cluster...")

	var tpl bytes.Buffer
	if err := template.Must(template.New("").Parse(OcpInstallerConfigTemplate)).Execute(&tpl, d.ctx); err != nil {
		return err
	}

	installConfig := filepath.Join(d.ctx["ClusterStateDir"].(string), "install-config.yaml")
	err := ioutil.WriteFile(installConfig, tpl.Bytes(), 0600)

	if err != nil {
		return err
	}

	cmd := NewCommand("openshift-install create cluster --dir {{.ClusterStateDir}}")
	err = cmd.AsTemplate(d.ctx).Run()

	// We want to *always* upload the state of the cluster
	// this way we can run a delete operation even on failed
	// deployments to clean all the resources on GCP.
	_ = d.uploadCredentials()
	return err
}

func (d *OcpDriver) uploadCredentials() error {
	// Let's check that the cluster dir exists
	// before we attempt an upload.
	if _, err := os.Stat(d.ctx["ClusterStateDir"].(string)); os.IsNotExist(err) {
		log.Printf("clusterStateDir %s not present", d.ctx["ClusterStateDir"])
		return nil
	}

	cmd := "gsutil mb gs://{{.OcpStateBucket}}"
	exists, err := NewCommand(cmd).AsTemplate(d.ctx).OutputContainsAny("already exists")

	if !exists && err != nil {
		log.Printf("error creating bucket gs://%s", d.ctx["OcpStateBucket"])
		log.Printf("%s", err)
		return err
	}

	log.Printf("uploading cluster state %s to gs://%s/%s", d.ctx["ClusterStateDir"], OcpStateBucket, d.ctx["ClusterName"])
	cmd = "gsutil rsync -r -d {{.ClusterStateDir}} gs://{{.OcpStateBucket}}/{{.ClusterName}}"
	return NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().Run()
}

type NoCredentials struct {
	err error
}

func (e *NoCredentials) Error() string {
	return "No credentials found"
}

func (e *NoCredentials) Unwrap() error {
	return e.err
}

func (d *OcpDriver) GetCredentials() error {
	cleanUp, err := d.ensureContext()
	defer cleanUp()
	if err != nil {
		return err
	}

	log.Printf("Getting credentials")
	kubeConfig := filepath.Join(d.ctx["ClusterStateDir"].(string), "auth", "kubeconfig")

	copyKubeconfig := func() error {
		if d.ctx["OverwriteDefaultKubeconfig"] == true {
			log.Printf("copying %s to ~/.kube/config", kubeConfig)
			if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".kube"), os.ModePerm); err != nil {
				return err
			}
			cmd := fmt.Sprintf("cp %s ~/.kube/config", kubeConfig)
			return NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().Run()
		}

		return nil
	}
	// We do this check twice to avoid re-downloading files
	// from the bucket when we already have them locally.
	// The second time is further down in this function and it's
	// done when the rsync succeeds
	if _, err := os.Stat(kubeConfig); !os.IsNotExist(err) {
		err = copyKubeconfig()
		if err != nil {
			return err
		}

		log.Printf("OpenShift's kubeconfig file exists and it's been copied under ~/.kube")
		return nil
	}

	cmd := "gsutil rsync -r -d gs://{{.OcpStateBucket}}/{{.ClusterName}} {{.ClusterStateDir}}"
	doesNotExist, err := NewCommand(cmd).AsTemplate(d.ctx).WithoutStreaming().OutputContainsAny("BucketNotFoundException")
	if err != nil || doesNotExist {
		// wrapping the error if any even though we are not logging it anymore as it adds to much noise to the output
		return &NoCredentials{err}
	}

	return copyKubeconfig()
}

func (d *OcpDriver) delete() error {
	log.Println("Deleting cluster...")

	cmd := NewCommand("openshift-install destroy cluster --dir {{.ClusterStateDir}}")
	err := cmd.AsTemplate(d.ctx).Run()

	if err != nil {
		return err
	}

	// No need to check whether this `rb` command succeeds
	_ = NewCommand("gsutil rm -r gs://{{.OcpStateBucket}}/{{.ClusterName}}").AsTemplate(d.ctx).WithoutStreaming().Run()
	return nil
}
