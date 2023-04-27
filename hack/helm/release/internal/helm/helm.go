// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	defaultElasticHelmRepo = "https://helm.elastic.co"

	timeout = 5 * time.Minute
)

// ReleaseConfig is the configuration needed to
// release all charts in a given directory.
type ReleaseConfig struct {
	// ChartsDir is the directory from which to release all Helm charts.
	ChartsDir string
	// Bucket is the GCS bucket to which to release all Helm charts.
	Bucket string
	// ChartsRepoURL is the Helm charts repository URL to which to release all Helm charts.
	ChartsRepoURL string
	// CredentialsFilePath is the path to the Google credentials JSON file.
	CredentialsFilePath string
	// DryRun determines whether to run the release without making any changes to the GCS bucket or the Helm repository index file.
	DryRun bool
}

// Release runs the Helm release.
func Release(conf ReleaseConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tempDir, err := os.MkdirTemp(os.TempDir(), "charts")
	if err != nil {
		return fmt.Errorf("while creating temp dir: %w", err)
	}
	//defer os.RemoveAll(tempDir)

	charts, err := readCharts(conf.ChartsDir)
	if err != nil {
		return fmt.Errorf("while reading charts: %w", err)
	}
	if err := uploadCharts(ctx, conf, tempDir, charts); err != nil {
		return fmt.Errorf("while uploading charts: %w", err)
	}
	if err := updateIndex(ctx, conf, tempDir); err != nil {
		return fmt.Errorf("while updating index: %w", err)
	}

	return nil
}

// readCharts reads all Helm charts in the given directory based on the presence of the Chart.yaml file.
func readCharts(dir string) ([]chart, error) {
	var charts []chart
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		matched, err := filepath.Match("Chart.yaml", filepath.Base(path))
		if err != nil {
			return err
		}
		if matched {
			fileBytes, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var ch chart
			if err = yaml.Unmarshal(fileBytes, &ch); err != nil {
				return err
			}
			ch.srcPath = filepath.Dir(path)
			charts = append(charts, ch)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return charts, nil
}

// uploadCharts
//  2. For each Helm chart, copy the chart to a temporary directory
//  3. For each Helm chart's dependency, if the dependency's repository
//     is set to 'https://helm.elastic.co', and the flag 'charts-repo-url'
//     is set to another repository (such as a dev helm repo), the dependency's
//     repository will be re-written to the given 'charts-repo-url' flag.
//  5. Run 'Helm package chart' for the Helm Chart to generate a tarball.
//  6. Copy the tarball to the GCS bucket.
func uploadCharts(ctx context.Context, conf ReleaseConfig, tempDir string, charts []chart) error {
	if len(charts) == 0 {
		return nil
	}

	for _, chart := range charts {
		// prepare a temporary directory for the chart
		tempChartDirPath := filepath.Join(tempDir, chart.Name)
		err := os.Mkdir(tempChartDirPath, 0755)
		if err != nil {
			return err
		}
		// copy the chart into this temporary directory
		err = copy(chart.srcPath, tempChartDirPath)
		if err != nil {
			return fmt.Errorf("while copying chart (%s) to temporary directory: %w", chart.Name, err)
		}

		if err := rewriteDependencyChartURLs(conf.ChartsRepoURL, tempChartDirPath); err != nil {
			return err
		}
		if err := copyChartDependencyPackage(chart, tempChartDirPath); err != nil {
			return err
		}

		// generate the chart .tgz package
		chartPackage := action.NewPackage()
		chartPackage.Destination = filepath.Join(tempDir, chart.Name)
		chartPackagePath, err := chartPackage.Run(tempChartDirPath, map[string]interface{}{})
		if err != nil {
			return fmt.Errorf("while packaging helm chart (%s): %w", chart.Name, err)
		}

		// upload the chart to the bucket
		if err := copyChartToGCSBucket(ctx, conf, chart.Name, chartPackagePath); err != nil {
			return err
		}
	}

	return nil
}

// copy will recursively copy a given source, to a given destination.
func copy(source, destination string) error {
	var err error = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		var relPath string = strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(destination, relPath), 0755)
		} else {
			var data, err = ioutil.ReadFile(filepath.Join(source, relPath))
			if err != nil {
				return err
			}
			return ioutil.WriteFile(filepath.Join(destination, relPath), data, 0777)
		}
	})
	return err
}

// rewriteDependencyChartURLs rewrites the URL of Helm chart's dependencies with the given repository URL.
// This is useful when charts are published to the dev Helm repo.
func rewriteDependencyChartURLs(repoURL string, chartPath string) error {
	chartYamlFilePath := filepath.Join(chartPath, "Chart.yaml")
	data, err := ioutil.ReadFile(chartYamlFilePath)
	if err != nil {
		return fmt.Errorf("while reading file (%s): %w", chartYamlFilePath, err)
	}

	// validate URL
	u, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("while parsing url (%s): %w", repoURL, err)
	}
	// strip potential trailing path
	rootURL := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	udpatedChartYaml := strings.ReplaceAll(
		string(data),
		fmt.Sprintf(`repository: "%s"`, defaultElasticHelmRepo),
		fmt.Sprintf(`repository: "%s"`, rootURL),
	)

	err = ioutil.WriteFile(chartYamlFilePath, []byte(udpatedChartYaml), 0)
	if err != nil {
		return fmt.Errorf("while writing (%s): %w", chartYamlFilePath, err)
	}
	return nil
}

// copyChartDependencies copies the package of each chart dependency of the given chart into its charts/ directory.
// This is the equivalent of 'helm dependency update chart'.
func copyChartDependencyPackage(chart chart, chartPath string) error {
	log.Printf("Copying dependencies for chart (%s)\n", chart.Name)
	for _, dep := range chart.Dependencies {
		if dep.Repository != "" {
			packageName := fmt.Sprintf("%s-%s.tgz", dep.Name, dep.Version)
			srcFile := filepath.Join(chartPath, "..", dep.Name, packageName)
			dstFile := filepath.Join(chartPath, "charts", packageName)
			fmt.Printf("cp %s %s\n", srcFile, dstFile)
			bytes, err := ioutil.ReadFile(srcFile)
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(dstFile, bytes, 0644)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// copyChartToGCSBucket copies a given Helm chart package to the GCS bucket.
// If the object already exists within the bucket, it is only overwritten
// if the chart is a SNAPSHOT release, otherwise an error is returned.
func copyChartToGCSBucket(ctx context.Context, conf ReleaseConfig, chartName, chartPackagePath string) error {
	repoURL, err := url.Parse(conf.ChartsRepoURL)
	if err != nil {
		return fmt.Errorf("while parsing url (%s): %w", conf.ChartsRepoURL, err)
	}

	// read the file to copy
	chartPackageFile, err := os.Open(chartPackagePath)
	if err != nil {
		return fmt.Errorf("while opening chart (%s): %w", chartPackagePath, err)
	}
	defer chartPackageFile.Close()

	destination := filepath.Join(repoURL.Path, chartName, filepath.Base(chartPackagePath))
	log.Printf("Writing chart to bucket path (%s)\n", destination)

	// create gcs client
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer gcsClient.Close()
	gcsBucket := gcsClient.Bucket(conf.Bucket).Object(destination)

	// allow overwrite only SNAPSHOT charts
	isSnapshot := strings.HasSuffix(chartPackagePath, "-SNAPSHOT.tgz")
	if !isSnapshot {
		gcsBucket = gcsBucket.If(storage.Conditions{DoesNotExist: true})
	}

	if conf.DryRun {
		log.Printf("not uploading (%s) to %s as dry-run is set", chartPackagePath, destination)
		return nil
	}

	// upload the file to the bucket
	gcsBucketWriter := gcsBucket.NewWriter(ctx)
	if _, err = io.Copy(gcsBucketWriter, chartPackageFile); err != nil {
		return fmt.Errorf("while copying data to bucket: %w", err)
	}
	if err := gcsBucketWriter.Close(); err != nil {
		switch errType := err.(type) {
		case *googleapi.Error:
			if errType.Code == http.StatusPreconditionFailed && !isSnapshot {
				return fmt.Errorf("file %s already exists in remote bucket; manually remove for this operation to succeed", chartPackagePath)
			}
			return fmt.Errorf("while writing data to bucket: %w", err)
		default:
			return fmt.Errorf("while writing data to bucket: %w", err)
		}
	}

	return nil
}

// updateIndex updates the Helm repo index by merging the existing index in the bucket
// with a new version created with the released charts.
func updateIndex(ctx context.Context, conf ReleaseConfig, tempDir string) error {
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer gcsClient.Close()

	// read existing index from the bucket and write it into a temp file
	oldIndexPath := filepath.Join(tempDir, "index.yaml.old")
	oldIndexFile, err := os.Create(oldIndexPath)
	if err != nil {
		return fmt.Errorf("while creating empty index.yaml.old: %w", err)
	}
	defer oldIndexFile.Close()
	gcsReader, err := gcsClient.Bucket(conf.Bucket).Object("index.yaml").NewReader(ctx)
	if err != nil {
		return fmt.Errorf("while creating new reader for index.yaml<xx: %w", err)
	}
	defer gcsReader.Close()

	if _, err := io.Copy(oldIndexFile, gcsReader); err != nil {
		return fmt.Errorf("while writing index.yaml.old: %w", err)
	}

	// generate the new index
	newIndex, err := repo.IndexDirectory(tempDir, conf.ChartsRepoURL)
	if err != nil {
		return fmt.Errorf("while indexing helm charts in temporary directory: %w", err)
	}
	// load the old index
	oldIndex, err := repo.LoadIndexFile(oldIndexPath)
	if err != nil {
		return fmt.Errorf("while loading existing helm index file: %w", err)
	}

	// merge the two indexes
	newIndex.Merge(oldIndex)
	newIndex.SortEntries()

	log.Printf("Writing new helm index file for %s", conf.ChartsRepoURL)

	// copy the new updated index into a temp file
	if err = newIndex.WriteFile(filepath.Join(tempDir, "index.yaml"), 0644); err != nil {
		return fmt.Errorf("while writing new index.yaml: %w", err)
	}
	newIndexFile, err := os.Open(filepath.Join(tempDir, "index.yaml"))
	if err != nil {
		return fmt.Errorf("while opening new index.yaml: %w", err)
	}
	defer newIndexFile.Close()

	if conf.DryRun {
		log.Printf("not uploading index as dry-run is set")
		return nil
	}

	// upload updated index file to the bucket
	gcsWriter := gcsClient.Bucket(conf.Bucket).Object("index.yaml").NewWriter(ctx)
	if _, err = io.Copy(gcsWriter, newIndexFile); err != nil {
		return fmt.Errorf("while copying new index.yaml to bucket: %w", err)
	}
	defer gcsWriter.Close()

	return nil
}
