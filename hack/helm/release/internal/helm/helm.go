// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/downloader"
	"helm.sh/helm/v4/pkg/registry"
	"helm.sh/helm/v4/pkg/repo/v1"
	"oras.land/oras-go/v2/errdef"
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
	// DryRun determines whether to run the release without making any changes to the GCS bucket, the Helm repository index file,
	// or the OCI registry. Reads, such as fetching the existing index or checking whether charts are already published, still happen.
	DryRun bool
	// Force determines if uploading charts should overwrite existing charts in the GCS bucket and the OCI registry even if they are not SNAPSHOT versions.
	Force bool
	// KeepTmpDir determines whether the temporary directory should be kept or not
	KeepTmpDir bool
	// OCIRegistry is the OCI registry to push Helm charts to (e.g. "docker.elastic.co/eck-charts").
	OCIRegistry string
	// OCIUsername and OCIPassword are the OCI registry credentials. When empty, the credentials from the local
	// Helm registry config or Docker config are used.
	OCIUsername string
	OCIPassword string
	// IsProdRelease indicates this is a production release. Charts cannot be overwritten in production
	// unless Force is set.
	IsProdRelease bool
	// SkipChartRepo skips the traditional HTTP chart repo channel (GCS upload + index update). Useful when only OCI publishing is needed.
	SkipChartRepo bool
	// SkipOCIRegistry skips pushing charts to the OCI registry. Useful when only GCS publishing is needed.
	SkipOCIRegistry bool
	// OCIChartsDigestsFilePath is an optional path to a file where pushed OCI chart digest refs are written,
	// one per line in the format "registry/chart:version@sha256:...". The file is truncated if it exists. Empty to skip.
	OCIChartsDigestsFilePath string
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

	if err := checkChartsVersions(conf.IsProdRelease, charts); err != nil {
		return err
	}

	packagedCharts, err := packageCharts(tempDir, charts)
	if err != nil {
		return fmt.Errorf("while packaging charts: %w", err)
	}

	if !conf.SkipChartRepo {
		if err := uploadChartsToGCS(ctx, conf, packagedCharts); err != nil {
			return fmt.Errorf("while uploading charts: %w", err)
		}

		if err := updateIndex(ctx, conf, tempDir); err != nil {
			return fmt.Errorf("while updating index: %w", err)
		}
	}

	if !conf.SkipOCIRegistry {
		ociClient, err := newOCIClient(conf)
		if err != nil {
			return fmt.Errorf("while creating OCI registry client: %w", err)
		}
		if err := pushChartsToOCI(ociClient, conf, packagedCharts); err != nil {
			return fmt.Errorf("while uploading charts to OCI registry: %w", err)
		}
	}

	return nil
}

// checkChartsVersions returns an error if any chart version has SemVer build metadata, or if this is a production
// release and any chart has a SNAPSHOT version.
// All offending charts are reported at once so the release fails before anything is packaged or published.
func checkChartsVersions(isProdRelease bool, charts []chart) error {
	var err error
	for _, c := range charts {
		// build metadata is not supported: "+" is invalid in OCI tags and it would hide the SNAPSHOT suffix
		if strings.Contains(c.Version, "+") {
			err = errors.Join(err, fmt.Errorf("chart (%s) has a version with build metadata (%s), which is not supported", c.Name, c.Version))
		}
		if isProdRelease && strings.HasSuffix(c.Version, "-SNAPSHOT") {
			err = errors.Join(err, fmt.Errorf("chart (%s) has a SNAPSHOT version (%s) and cannot be released to production", c.Name, c.Version))
		}
	}
	return err
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

// packageCharts packages each chart into a chart archive in the given temporary directory.
func packageCharts(tempDir string, charts []chart) ([]packagedChart, error) {
	packagedCharts := make([]packagedChart, 0, len(charts))
	for _, chart := range charts {
		// prepare a temp directory for the chart sources
		tempChartDirPath := filepath.Join(tempDir, chart.Name)
		err := os.Mkdir(tempChartDirPath, 0755)
		if err != nil {
			return nil, err
		}
		// copy the chart sources into this temp directory
		err = copy(chart.srcPath, tempChartDirPath)
		if err != nil {
			return nil, fmt.Errorf("while copying chart (%s) to temporary directory: %w", chart.Name, err)
		}

		// generates Chart.lock by doing the equivalent of 'helm update dependency', which will not download or update anything
		// as all dependencies are local without repository
		man := &downloader.Manager{Out: io.Discard, ChartPath: tempChartDirPath}
		if err := man.Update(); err != nil {
			return nil, fmt.Errorf("while updating chart (%s) dependencies to generate Chart.lock: %w", chart.Name, err)
		}

		// package the chart into a chart archive
		chartPackage := action.NewPackage()
		chartPackage.Destination = filepath.Join(tempDir, chart.Name)
		chartPackagePath, err := chartPackage.Run(tempChartDirPath, map[string]any{})
		if err != nil {
			return nil, fmt.Errorf("while packaging helm chart (%s): %w", chart.Name, err)
		}
		packagedCharts = append(packagedCharts, packagedChart{
			chart:       chart,
			packagePath: chartPackagePath,
		})
	}
	return packagedCharts, nil
}

// copy copies a given source to a given destination.
func copy(source, destination string) error {
	walkErr := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		relPath := strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(destination, relPath), 0755)
		}

		data, err := os.ReadFile(filepath.Join(source, relPath))
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(destination, relPath), data, 0777)
	})
	return walkErr
}

// uploadChartsToGCS uploads the packaged chart archives to the GCS bucket.
func uploadChartsToGCS(ctx context.Context, conf ReleaseConfig, charts []packagedChart) error {
	repoURL, err := url.Parse(conf.ChartsRepoURL)
	if err != nil {
		return fmt.Errorf("while parsing url (%s): %w", conf.ChartsRepoURL, err)
	}
	// trail the first / from the repo url path
	repoPath := strings.TrimPrefix(repoURL.Path, "/")

	// create gcs client
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer gcsClient.Close()
	bucket := gcsClient.Bucket(conf.Bucket)

	for _, chart := range charts {
		if err := copyChartToGCSBucket(ctx, conf, bucket, repoPath, chart); err != nil {
			return err
		}
	}
	return nil
}

// copyChartToGCSBucket copies a given chart archive to the GCS bucket.
// Charts cannot be overwritten in the prod bucket unless forced, otherwise an error is returned.
func copyChartToGCSBucket(ctx context.Context, conf ReleaseConfig, bucket *storage.BucketHandle, repoPath string, chart packagedChart) error {
	// read the file to copy on disk
	chartPackageFile, err := os.Open(chart.packagePath)
	if err != nil {
		return fmt.Errorf("while opening chart (%s): %w", chart.packagePath, err)
	}
	defer chartPackageFile.Close()

	chartArchiveDest := filepath.Join(repoPath, chart.Name, filepath.Base(chart.packagePath))

	log.Printf("Writing chart archive to bucket path (%s)\n", chartArchiveDest)

	chartArchiveObj := bucket.Object(chartArchiveDest)

	// specify that the object must not exist when publishing to prod Helm repo
	shouldNotOverwrite := conf.shouldNotOverwrite()
	if shouldNotOverwrite {
		chartArchiveObj = chartArchiveObj.If(storage.Conditions{DoesNotExist: true})
	}

	if conf.DryRun {
		log.Printf("Not uploading (%s) to %s as dry-run is set", chart.packagePath, chartArchiveDest)
		return nil
	}

	// upload the file to the bucket
	chartArchiveWriter := chartArchiveObj.NewWriter(ctx)
	if _, err = io.Copy(chartArchiveWriter, chartPackageFile); err != nil {
		return fmt.Errorf("while copying data to bucket: %w", err)
	}
	if err := chartArchiveWriter.Close(); err != nil {
		if errType, ok := errors.AsType[*googleapi.Error](err); ok {
			if errType.Code == http.StatusPreconditionFailed && shouldNotOverwrite {
				return fmt.Errorf("file %s already exists in remote bucket; manually remove for this operation to succeed", chart.packagePath)
			}
		}
		return fmt.Errorf("while writing data to bucket: %w", err)
	}
	return nil
}

// shouldNotOverwrite determines if charts should not be overwritten in the bucket or the OCI registry.
// Prod releases contain only non-SNAPSHOT charts, as enforced by checkChartsVersions.
func (conf ReleaseConfig) shouldNotOverwrite() bool {
	return conf.IsProdRelease && !conf.Force
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
	if err := newIndexObjWriter.Close(); err != nil {
		return fmt.Errorf("while finalizing upload of index.yaml: %w", err)
	}

	return nil
}

// pushChartsToOCI pushes the packaged chart archives to the OCI registry.
// Charts cannot be overwritten in the prod registry unless forced, otherwise an error is returned.
// If conf.OCIChartsDigestsFilePath is set, the digest ref of each pushed chart is written to it in the format
// "registry/chart:version@sha256:...", one line per pushed chart.
func pushChartsToOCI(client ociPusher, conf ReleaseConfig, charts []packagedChart) error {
	digestsFileWriter := io.Discard
	if outputDigestsFilePath := conf.OCIChartsDigestsFilePath; outputDigestsFilePath != "" {
		f, err := os.OpenFile(outputDigestsFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("while opening OCI charts digests file (%s): %w", outputDigestsFilePath, err)
		}
		defer f.Close()
		digestsFileWriter = f
	}

	for _, chart := range charts {
		if err := pushChartToOCI(client, conf, digestsFileWriter, chart); err != nil {
			return err
		}
	}
	return nil
}

// newOCIClient creates an OCI registry client, using the credentials from conf if set.
func newOCIClient(conf ReleaseConfig) (*registry.Client, error) {
	// the registry client doesn't take a context, so the timeout is enforced per HTTP request instead
	ociClientOpts := []registry.ClientOption{
		registry.ClientOptHTTPClient(&http.Client{
			Transport: registry.NewTransport(false),
			Timeout:   timeout,
		}),
	}
	if conf.OCIUsername != "" && conf.OCIPassword != "" {
		ociClientOpts = append(ociClientOpts, registry.ClientOptBasicAuth(conf.OCIUsername, conf.OCIPassword))
	}
	return registry.NewClient(ociClientOpts...)
}

func pushChartToOCI(client ociPusher, conf ReleaseConfig, outputDigestsFileWriter io.Writer, chart packagedChart) error {
	chartRef := fmt.Sprintf("%s/%s:%s", conf.OCIRegistry, chart.Name, chart.Version)

	// check that the chart does not already exist when publishing to prod OCI registry
	if conf.shouldNotOverwrite() {
		_, err := client.Resolve(chartRef)
		switch {
		case err == nil:
			return fmt.Errorf("chart (%s) already exists in OCI registry (%s); remove it or use --force to overwrite", chartRef, conf.OCIRegistry)
		case errors.Is(err, errdef.ErrNotFound):
		// chart doesn't exist
		default:
			return fmt.Errorf("while checking if chart (%s) already exists in OCI registry (%s): %w", chartRef, conf.OCIRegistry, err)
		}
	}

	if conf.DryRun {
		log.Printf("Not pushing chart (%s) to OCI registry as dry-run is set", chartRef)
		return nil
	}

	log.Printf("Pushing chart (%s) to OCI registry\n", chartRef)
	chartBytes, err := os.ReadFile(chart.packagePath)
	if err != nil {
		return fmt.Errorf("while reading chart archive (%s): %w", chart.packagePath, err)
	}

	result, err := client.Push(chartBytes, chartRef)
	if err != nil {
		return fmt.Errorf("while pushing chart (%s) to OCI registry: %w", chartRef, err)
	}
	log.Printf("Pushed chart (%s) to OCI registry: digest=%s", chartRef, result.Manifest.Digest)

	digestRef := fmt.Sprintf("%s@%s", chartRef, result.Manifest.Digest)
	if _, err := fmt.Fprintln(outputDigestsFileWriter, digestRef); err != nil {
		return fmt.Errorf("while writing OCI digest ref for chart (%s): %w", chart.Name, err)
	}
	return nil
}
