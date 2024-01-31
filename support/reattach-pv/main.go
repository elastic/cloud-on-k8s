// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	esManifestFlag = "elasticsearch-manifest"
	oldEsNameFlag  = "old-elasticsearch-name"
	dryRunFlag     = "dry-run"
)

var Cmd = &cobra.Command{
	Use:   "reattach-pv",
	Short: "Recreate an Elasticsearch cluster by reattaching existing released PersistentVolumes",
	Run: func(cmd *cobra.Command, args []string) {
		dryRun := viper.GetBool(dryRunFlag)
		if dryRun {
			fmt.Println("Running in dry run mode")
		}

		err := esv1.AddToScheme(scheme.Scheme)
		exitOnErr(err)

		es, err := esFromFile(viper.GetString(esManifestFlag))
		exitOnErr(err)

		c, err := createClient()
		exitOnErr(err)

		err = checkElasticsearchNotFound(c, es)
		exitOnErr(err)

		expectedClaims := expectedVolumeClaims(es)
		err = checkClaimsNotFound(c, expectedClaims)
		exitOnErr(err)

		releasedPVs, err := findReleasedPVs(c)
		exitOnErr(err)

		matches, err := matchPVsWithClaim(releasedPVs, expectedClaims, es, viper.GetString(oldEsNameFlag))
		exitOnErr(err)

		err = createAndBindClaims(c, matches, dryRun)
		exitOnErr(err)

		err = createElasticsearch(c, es, dryRun)
		exitOnErr(err)
	},
}

func init() {
	Cmd.Flags().String(
		esManifestFlag,
		"",
		"path pointing to the Elasticsearch yaml manifest",
	)
	Cmd.Flags().String(
		oldEsNameFlag,
		"",
		"name of previous Elasticsearch cluster (to use existing volumes)",
	)
	Cmd.Flags().Bool(
		dryRunFlag,
		false,
		"do not apply any Kubernetes resource change",
	)
	exitOnErr(viper.BindPFlags(Cmd.Flags()))
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func main() {
	exitOnErr(Cmd.Execute())
}

// esFromFile parses an Elasticsearch resource from the given yaml manifest path.
func esFromFile(path string) (esv1.Elasticsearch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return esv1.Elasticsearch{}, err
	}
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(data, nil, nil)
	if err != nil {
		return esv1.Elasticsearch{}, err
	}
	es, ok := obj.(*esv1.Elasticsearch)
	if !ok {
		return esv1.Elasticsearch{}, fmt.Errorf("cannot serialize content of %s into an Elasticsearch object", path)
	}
	if es.Namespace == "" {
		fmt.Println("Setting Elasticsearch namespace to 'default'")
		es.Namespace = "default"
	}
	return *es, nil
}

// createClient creates a Kubernetes client targeting the current default K8s cluster.
func createClient() (k8s.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// checkElasticsearchNotFound returns an error if the given Elasticsearch resource already exists.
func checkElasticsearchNotFound(c k8s.Client, es esv1.Elasticsearch) error {
	var retrieved esv1.Elasticsearch
	err := c.Get(context.Background(), k8s.ExtractNamespacedName(&es), &retrieved)
	if err == nil {
		return fmt.Errorf("elasticsearch resource %s exists in the apiserver; this tool can only recover clusters that don't exist anymore", es.Name)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// checkClaimsNotFound returns an error if the given PersistentVolumeClaims already exist.
func checkClaimsNotFound(c k8s.Client, claims map[types.NamespacedName]v1.PersistentVolumeClaim) error {
	for nsn := range claims {
		err := c.Get(context.Background(), nsn, &v1.PersistentVolumeClaim{})
		if err == nil {
			return fmt.Errorf("PersistentVolumeClaim %s seems to exist in the apiserver", nsn)
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// expectedVolumeClaims builds the list of PersistentVolumeClaim that we expect to exist for the given
// Elasticsearch cluster.
func expectedVolumeClaims(es esv1.Elasticsearch) map[types.NamespacedName]v1.PersistentVolumeClaim {
	claims := make(map[types.NamespacedName]v1.PersistentVolumeClaim, es.Spec.NodeCount())
	for _, nodeSet := range es.Spec.NodeSets {
		for i := int32(0); i < nodeSet.Count; i++ {
			var claim v1.PersistentVolumeClaim
			for _, claimTemplate := range nodeSet.VolumeClaimTemplates {
				if claimTemplate.Name == volume.ElasticsearchDataVolumeName {
					claim = claimTemplate
				}
			}
			claim.Name = fmt.Sprintf(
				"%s-%s",
				volume.ElasticsearchDataVolumeName,
				sset.PodName(esv1.StatefulSet(es.Name, nodeSet.Name), i))
			claim.Namespace = es.Namespace
			if claim.Namespace == "" {
				claim.Namespace = "default"
			}
			// simulate a bound status
			claim.Status = v1.PersistentVolumeClaimStatus{
				Phase:       v1.ClaimBound,
				AccessModes: claim.Spec.AccessModes,
				Capacity:    claim.Spec.Resources.Requests,
			}
			claims[types.NamespacedName{Namespace: es.Namespace, Name: claim.Name}] = claim
			fmt.Printf("Expecting claim %s\n", claim.Name)
		}
	}
	return claims
}

// findReleasedPVs returns the list of Released PersistentVolumes.
func findReleasedPVs(c k8s.Client) ([]v1.PersistentVolume, error) {
	var pvs v1.PersistentVolumeList
	if err := c.List(context.Background(), &pvs); err != nil {
		return nil, err
	}
	var released []v1.PersistentVolume
	for _, pv := range pvs.Items {
		if pv.Status.Phase == v1.VolumeReleased {
			released = append(released, pv)
		}
	}
	fmt.Printf("Found %d released PersistentVolumes\n", len(pvs.Items))
	return released, nil
}

// MatchingVolumeClaim matches an existing PersistentVolume with a new PersistentVolumeClaim.
type MatchingVolumeClaim struct {
	claim  v1.PersistentVolumeClaim
	volume v1.PersistentVolume
}

// matchPVsWithClaim iterates over existing pvs to match them to an expected pvc.
func matchPVsWithClaim(pvs []v1.PersistentVolume, claims map[types.NamespacedName]v1.PersistentVolumeClaim, es esv1.Elasticsearch, oldESName string) ([]MatchingVolumeClaim, error) {
	matches := make([]MatchingVolumeClaim, 0, len(pvs))
	for _, pv := range pvs {
		if pv.Spec.ClaimRef == nil {
			continue
		}
		expectedClaimName := pv.Spec.ClaimRef.Name
		// if you're building a newly named cluster, from a previous cluster's PVs, we'll
		// need to replace the old cluster's name, with the new cluster's name to try and match
		// against the set of generated claims.
		if oldESName != "" {
			expectedClaimName = strings.ReplaceAll(expectedClaimName, pvcPrefix(oldESName), pvcPrefix(es.Name))
		}

		claim, expected := claims[types.NamespacedName{Namespace: pv.Spec.ClaimRef.Namespace, Name: expectedClaimName}]
		if !expected {
			continue
		}
		fmt.Printf("Found matching volume %s for claim %s\n", pv.Name, claim.Name)
		matches = append(matches, MatchingVolumeClaim{
			claim:  claim,
			volume: pv,
		})
	}
	if len(matches) != len(claims) {
		return nil, fmt.Errorf("found %d matching volumes but expected %d", len(matches), len(claims))
	}
	return matches, nil
}

func pvcPrefix(clusterName string) string {
	return esv1.ESNamer.Suffix(fmt.Sprintf("%s-%s", volume.ElasticsearchDataVolumeName, clusterName))
}

// bindNewClaims creates the given PersistentVolumeClaims, and update the matching PersistentVolumes
// to reference the claim.
func createAndBindClaims(c k8s.Client, volumeClaims []MatchingVolumeClaim, dryRun bool) error {
	for _, match := range volumeClaims {
		match := match
		fmt.Printf("Creating claim %s\n", match.claim.Name)
		if !dryRun {
			if err := c.Create(context.Background(), &match.claim); err != nil {
				return err
			}
		}
		fmt.Printf("Updating volume %s to reference claim %s\n", match.volume.Name, match.claim.Name)
		// match.claim now stores the created claim metadata
		// patch the volume spec to match the new claim
		match.volume.Spec.ClaimRef.UID = match.claim.UID
		match.volume.Spec.ClaimRef.Name = match.claim.Name
		match.volume.Spec.ClaimRef.ResourceVersion = match.claim.ResourceVersion
		if !dryRun {
			if err := c.Update(context.Background(), &match.volume); err != nil {
				return err
			}
		}
	}
	return nil
}

// createElasticsearch creates the given Elasticsearch resource.
func createElasticsearch(c k8s.Client, es esv1.Elasticsearch, dryRun bool) error {
	fmt.Printf("Creating Elasticsearch %s\n", es.Name)
	if dryRun {
		return nil
	}
	return c.Create(context.Background(), &es, &client.CreateOptions{})
}

// exitOnErr prints the given error then exits with status code 1
func exitOnErr(err error) {
	if err != nil {
		println("Fatal error:", err.Error())
		os.Exit(1)
	}
}
