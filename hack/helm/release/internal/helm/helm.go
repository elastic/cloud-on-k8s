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
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	timeout = 5 * time.Minute
)

// ReleaseConfig is the configuration needed to release all charts in a given directory.
type ReleaseConfig struct {
	// ChartsDir is the directory from which to release Helm charts.
	ChartsDir string
	// Bucket is the GCS bucket to which to release Helm charts.
	Bucket string
	// ChartsRepoURL is the Helm charts repository URL to which to release Helm charts.
	ChartsRepoURL string
	// CredentialsFilePath is the path to the Google credentials JSON file.
	CredentialsFilePath string
	// DryRun determines whether to run the release without making any changes to the GCS bucket or the Helm repository index file.
	DryRun bool
	// KeepTmpDir determines whether the temporary directory should be kept or not
	KeepTmpDir bool
}

// Release runs the Helm charts release.
func Release(conf ReleaseConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tempDir, err := os.MkdirTemp(os.TempDir(), "charts")
	if err != nil {
		return fmt.Errorf("while creating temp dir: %w", err)
	}
	if conf.KeepTmpDir {
		log.Printf("Not deleting temporary directory: %s", tempDir)
	} else {
		defer os.RemoveAll(tempDir)
	}

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

// uploadCharts packages a chart into a chart archive and upload it to the GCS bucket.
func uploadCharts(ctx context.Context, conf ReleaseConfig, tempDir string, charts []chart) error {
	for _, chart := range charts {
		// prepare a temp directory for the chart sources
		tempChartDirPath := filepath.Join(tempDir, chart.Name)
		err := os.Mkdir(tempChartDirPath, 0755)
		if err != nil {
			return err
		}
		// copy the chart sources into this temp directory
		err = copy(chart.srcPath, tempChartDirPath)
		if err != nil {
			return fmt.Errorf("while copying chart (%s) to temporary directory: %w", chart.Name, err)
		}

		// generates Chart.lock by doing the equivalent of 'helm update dependency', which will not download or update anything
		// as all dependencies are local without repository
		man := &downloader.Manager{Out: ioutil.Discard, ChartPath: tempChartDirPath}
		if err := man.Update(); err != nil {
			return fmt.Errorf("while updating chart (%s) dependencies to generate Chart.lock: %w", chart.Name, err)
		}

		// package the chart into a chart archive
		chartPackage := action.NewPackage()
		chartPackage.Destination = filepath.Join(tempDir, chart.Name)
		chartPackagePath, err := chartPackage.Run(tempChartDirPath, map[string]interface{}{})
		if err != nil {
			return fmt.Errorf("while packaging helm chart (%s): %w", chart.Name, err)
		}
		// upload the chart archive to the bucket
		if err := copyChartToGCSBucket(ctx, conf, chart, chartPackagePath); err != nil {
			return err
		}
	}
	return nil
}

// copy copies a given source to a given destination.
func copy(source, destination string) error {
	var err error = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		relPath := strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(destination, relPath), 0755)
		} else {
			var data, err = os.ReadFile(filepath.Join(source, relPath))
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(destination, relPath), data, 0777)
		}
	})
	return err
}

// copyChartToGCSBucket copies a given chart archive to the GCS bucket.
// Only SNAPSHOT charts can be overwritten, otherwise an error is returned.
func copyChartToGCSBucket(ctx context.Context, conf ReleaseConfig, chart chart, chartPackagePath string) error {
	repoURL, err := url.Parse(conf.ChartsRepoURL)
	if err != nil {
		return fmt.Errorf("while parsing url (%s): %w", conf.ChartsRepoURL, err)
	}

	// read the file to copy on disk
	chartPackageFile, err := os.Open(chartPackagePath)
	if err != nil {
		return fmt.Errorf("while opening chart (%s): %w", chartPackagePath, err)
	}
	defer chartPackageFile.Close()

	// trail the first / from the repo url path
	chartArchiveDest := filepath.Join(strings.TrimPrefix(repoURL.Path, "/"), chart.Name, filepath.Base(chartPackagePath))

	log.Printf("Writing chart archive to bucket path (%s)\n", chartArchiveDest)

	// create gcs client
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer gcsClient.Close()
	chartArchiveObj := gcsClient.Bucket(conf.Bucket).Object(chartArchiveDest)

	// specify that the object must not exist for non-SNAPSHOT chart when publishing to prod Helm repo
	isNonSnapshot := !strings.HasSuffix(chart.Version, "-SNAPSHOT")
	isProdHelmRepo := !strings.HasSuffix(conf.Bucket, "-dev")
	if isNonSnapshot && isProdHelmRepo {
		chartArchiveObj = chartArchiveObj.If(storage.Conditions{DoesNotExist: true})
	}

	if conf.DryRun {
		log.Printf("Not uploading (%s) to %s as dry-run is set", chartPackagePath, chartArchiveDest)
		return nil
	}

	// upload the file to the bucket
	chartArchiveWriter := chartArchiveObj.NewWriter(ctx)
	if _, err = io.Copy(chartArchiveWriter, chartPackageFile); err != nil {
		return fmt.Errorf("while copying data to bucket: %w", err)
	}
	if err := chartArchiveWriter.Close(); err != nil {
		switch errType := err.(type) {
		case *googleapi.Error:
			if errType.Code == http.StatusPreconditionFailed && isNonSnapshot && isProdHelmRepo {
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
// with a new version created with the released charts. A 'GenerationMatch' precondition
// is used when writing to avoid a race condition with another concurrent write.
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
	oldIndexObj := gcsClient.Bucket(conf.Bucket).Object("index.yaml")
	oldIndexAttrs, err := oldIndexObj.Attrs(ctx)
	if err != nil {
		return fmt.Errorf("reading attributes of index.yaml: %w", err)
	}
	oldIndexObjReader, err := oldIndexObj.NewReader(ctx)
	if err != nil {
		return fmt.Errorf("while creating new reader for index.yaml: %w", err)
	}
	defer oldIndexObjReader.Close()

	if _, err := io.Copy(oldIndexFile, oldIndexObjReader); err != nil {
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
		log.Printf("Not uploading index.yaml as dry-run is set")
		return nil
	}

	// upload updated index file to the bucket
	newIndexObj := gcsClient.Bucket(conf.Bucket).Object("index.yaml")
	// prevent race condition where the file is overwritten by another concurrent release
	newIndexObj = newIndexObj.If(storage.Conditions{GenerationMatch: oldIndexAttrs.Generation})
	newIndexObjWriter := newIndexObj.NewWriter(ctx)
	if _, err = io.Copy(newIndexObjWriter, newIndexFile); err != nil {
		return fmt.Errorf("while copying new index.yaml to bucket: %w", err)
	}
	defer newIndexObjWriter.Close()

	return nil
}
