package helm

import (
	"context"
	"crypto/tls"
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
	"google.golang.org/api/option"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

const (
	googleCredentialsEnvVar = "GOOGLE_APPLICATION_CREDENTIALS"
	defaultGCSURL           = "https://storage.googleapis.com"
	defaultElasticHelmRepo  = "https://helm.elastic.co"
	chartYamlGlob           = "*/Chart.yaml"
	stableHelmChartsURL     = "https://charts.helm.sh/stable"
)

type ReleaseConfig struct {
	ChartsDir, Bucket, ChartsRepoURL string
	CredentialsFilePath              string
	UploadIndex                      bool
	DryRun                           bool
	GCSURL                           string
	Excludes                         []string
}

func Release(conf ReleaseConfig) error {
	return internalRelease(conf)
}

func internalRelease(conf ReleaseConfig) error {
	log.Println("Removing existing releases.")
	if err := removeExistingReleases(conf.ChartsDir); err != nil {
		return err
	}
	if err := ensureCredentialsFile(conf.CredentialsFilePath); err != nil {
		return err
	}
	charts, err := readCharts(conf.ChartsDir, conf.Excludes)
	if err != nil {
		return err
	}
	noDeps, withDeps := separateChartsWithDependencies(charts)
	tempDir, err := os.MkdirTemp(os.TempDir(), "charts")
	if err != nil {
		return fmt.Errorf("while creating temporary directory for charts: %w", err)
	}
	defer os.RemoveAll(tempDir)

	return uploadChartsAndUpdateIndex(uploadChartsConfig{
		releaseConf:   conf,
		tempDirectory: tempDir,
		noDeps:        noDeps,
		withDeps:      withDeps,
	})
}

func ensureCredentialsFile(path string) error {
	_, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("while reading google credentials file (%s): %w", path, err)
	}
	return os.Setenv(googleCredentialsEnvVar, path)
}

func removeExistingReleases(chartsDir string) error {
	// Cleanup existing release tarballs
	files, _ := filepath.Glob(chartsDir + "*/*.tgz")
	for _, file := range files {
		log.Printf("removing existing release: %s", file)
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("while removing release file (%s): %w", file, err)
		}
	}
	return nil
}

func readCharts(chartsDir string, excludes []string) ([]chart, error) {
	cs, err := filepath.Glob(filepath.Join(chartsDir, chartYamlGlob))
	if err != nil {
		return nil, fmt.Errorf("while searching for files matching pattern (%s): %w", chartYamlGlob, err)
	}
	charts := make([]chart, len(cs))
	for i, fullChartPath := range cs {
		fileBytes, err := os.ReadFile(fullChartPath)
		if err != nil {
			return nil, fmt.Errorf("while reading %s: %w", fullChartPath, err)
		}
		var ch chart
		if err = yaml.Unmarshal(fileBytes, &ch); err != nil {
			return nil, fmt.Errorf("while unmarshaling %s to chart: %w", fullChartPath, err)
		}
		if contains(ch.Name, excludes) {
			log.Printf("Excluding %s as it is in the excludes list", ch.Name)
			continue
		}
		charts[i] = ch
	}
	return charts, nil
}

func separateChartsWithDependencies(charts charts) (noDeps charts, withDeps charts) {
	var temp []chart
	for _, ch := range charts {
		if len(ch.Dependencies) == 0 {
			noDeps = append(noDeps, ch)
			continue
		}
		temp = append(temp, ch)
	}
	for _, ch := range temp {
		foundInDeps := false
		for _, dep := range ch.Dependencies {
			if contains(dep.Name, noDeps.chartNames()) {
				withDeps = append(withDeps, ch)
				foundInDeps = true
				break
			}
		}
		if !foundInDeps {
			noDeps = append(noDeps, ch)
		}
	}
	return
}

func contains(name string, names []string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

type uploadChartsConfig struct {
	releaseConf   ReleaseConfig
	tempDirectory string
	noDeps        charts
	withDeps      charts
}

func uploadChartsAndUpdateIndex(conf uploadChartsConfig) error {
	// upload charts without dependencies
	if err := uploadCharts(conf.tempDirectory, conf.noDeps, conf.releaseConf); err != nil {
		return err
	}
	if conf.releaseConf.UploadIndex {
		if err := updateIndex(conf); err != nil {
			return err
		}
	}
	if !conf.releaseConf.UploadIndex || conf.releaseConf.DryRun {
		// If the helm index isn't being updated, then we can't process charts with direct dependencies.
		log.Printf("Not processing charts with dependencies (%v) as the Helm index isn't being updated, or dry-run is set", conf.withDeps)
		return nil
	}
	// upload charts with dependencies
	if err := uploadCharts(conf.tempDirectory, conf.withDeps, conf.releaseConf); err != nil {
		return err
	}
	// update index without checking flag, as we've previously checked it.
	if err := updateIndex(conf); err != nil {
		return err
	}
	return nil
}

func uploadCharts(tempDir string, charts []chart, conf ReleaseConfig) error {
	settings := cli.New()
	log.Printf("Adding %s repository.\n", stableHelmChartsURL)
	// helm repo add stable https://charts.helm.sh/stable
	if _, err := repo.NewChartRepository(&repo.Entry{
		Name: "stable",
		URL:  stableHelmChartsURL,
	}, getter.All(settings)); err != nil {
		return fmt.Errorf("while adding helm stable charts repository: %w", err)
	}
	log.Printf("Done adding %s repository.\n", stableHelmChartsURL)
	for _, chart := range charts {
		chartPath := filepath.Join(tempDir, chart.Name)
		err := os.Mkdir(chartPath, 0755)
		if err != nil {
			return fmt.Errorf("while making chart directory inside temporary directory %s: %w", chartPath, err)
		}
		err = copy(filepath.Join(conf.ChartsDir, chart.Name), chartPath)
		if err != nil {
			return fmt.Errorf("while copying chart (%s) to temporary directory: %w", chart.Name, err)
		}

		log.Printf("Attempting to add chart (%s)\n", chart.Name)
		if err := updateDependencyChartURLs(chartPath, conf.ChartsRepoURL); err != nil {
			return err
		}
		// helm dependency update charts_dir/chart
		client := action.NewDependency()
		cfg := action.Configuration{}
		man := &downloader.Manager{
			Out:              os.Stdout,
			ChartPath:        chartPath,
			Keyring:          client.Keyring,
			SkipUpdate:       client.SkipRefresh,
			Getters:          getter.All(settings),
			RegistryClient:   cfg.RegistryClient,
			RepositoryConfig: settings.RepositoryConfig,
			RepositoryCache:  settings.RepositoryCache,
			Debug:            settings.Debug,
			Verify:           downloader.VerifyNever,
		}
		if client.Verify {
			man.Verify = downloader.VerifyAlways
		}
		log.Printf("Updating dependencies for chart (%s)\n", chart.Name)
		if err := man.Update(); err != nil {
			return fmt.Errorf("while updating dependencies for helm chart (%s): %w", chart.Name, err)
		}
		// helm package "charts_dir/chart" --destination "chart"
		packageClient := action.NewPackage()
		log.Printf("Packaging chart (%s)\n", chart.Name)
		if _, err := packageClient.Run(chartPath, map[string]interface{}{}); err != nil {
			return fmt.Errorf("while packaging helm chart (%s): %w", chart.Name, err)
		}
		source := fmt.Sprintf("%s-*.tgz", chart.Name)

		if err = copyChartToGCSBucket(); err != nil {
			return err
		}

	}

	return nil
}

type copyChartToBucketConfig struct{}

func copyChartToGCSBucket(ctx context.Context) error {
	// gsutil cp -n source destination
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	storageClient, err := getGCSClient(ctx, conf.GCSURL)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer storageClient.Close()

	files, err := filepath.Glob(source)
	if err != nil {
		return fmt.Errorf("while search for file glob (%s): %w", source, err)
	}
	if len(files) == 0 {
		return fmt.Errorf("couldn't file helm package with glob (%s)", source)
	}
	destination := fmt.Sprintf("helm/%s/%s", chart.Name, files[0])
	log.Printf("Writing chart to bucket path (%s) \n", destination)
	// Open Chart
	f, err := os.Open(files[0])
	if err != nil {
		return fmt.Errorf("while opening chart (%s): %w", files[0], err)
	}
	defer f.Close()
	defer func() {
		// intentionally ignoring failure to remove temporary *.tgz file
		_ = os.Remove(files[0])
	}()

	if conf.DryRun {
		log.Printf("not uploading (%s) as dry-run is set", files[0])
		continue
	}

	bkt := storageClient.Bucket(conf.Bucket)

	o := bkt.Object(destination)

	// Optional: set a generation-match precondition to avoid potential race
	// conditions and data corruptions. The request to upload is aborted if the
	// object's generation number does not match your precondition.
	// For an object that does not yet exist, set the DoesNotExist precondition.
	o = o.If(storage.Conditions{DoesNotExist: true})
	// If the live object already exists in your bucket, set instead a
	// generation-match precondition using the live object's generation number.
	// attrs, err := o.Attrs(ctx)
	// if err != nil {
	// 	return fmt.Errorf("object.Attrs: %v", err)
	// }
	// o = o.If(storage.Conditions{GenerationMatch: attrs.Generation})

	// Upload an object with storage.Writer.
	wc := o.NewWriter(ctx)
	if _, err = io.Copy(wc, f); err != nil {
		return fmt.Errorf("while copying data to bucket: %w", err)
	}

	if err := wc.Close(); err != nil {
		switch ee := err.(type) {
		case *googleapi.Error:
			if ee.Code == http.StatusPreconditionFailed {
				// The object already exists; this error is expected
				break
			}
			return fmt.Errorf("while writing data to bucket: %w", err)
		default:
			return fmt.Errorf("while writing data to bucket: %w", err)
		}
	}
	data, err := ioutil.ReadFile(files[0])
	if err != nil {
		return fmt.Errorf("while reading %s: %w", files[0], err)
	}

	err = ioutil.WriteFile(filepath.Join(tempDir, chart.Name, files[0]), data, 0777)
	if err != nil {
		return fmt.Errorf("while writing %s to temp directory: %w", files[0], err)
	}
	return nil
}

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

func updateDependencyChartURLs(chartPath string, repoURL string) error {
	chartYamlFilePath := filepath.Join(chartPath, "Chart.yaml")
	data, err := ioutil.ReadFile(chartYamlFilePath)
	if err != nil {
		return fmt.Errorf("while reading file (%s): %w", chartYamlFilePath, err)
	}
	u, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("while parsing url (%s): %w", repoURL, err)
	}
	// repoURL potentially has trailing path (https://helm-dev.elastic.co/helm)
	// this strips the trailing path, and contains only the scheme://host
	url := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	newContents := strings.ReplaceAll(
		string(data),
		fmt.Sprintf(`repository: "%s"`, defaultElasticHelmRepo),
		fmt.Sprintf(`repository: "%s"`, url),
	)

	err = ioutil.WriteFile(chartYamlFilePath, []byte(newContents), 0)
	if err != nil {
		return fmt.Errorf("while writing (%s): %w", chartYamlFilePath, err)
	}
	return nil
}

func getGCSClient(ctx context.Context, url string) (*storage.Client, error) {
	if url == defaultGCSURL || url == "" {
		return storage.NewClient(ctx)
	}
	os.Setenv("STORAGE_EMULATOR_HOST", url)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: tr}
	return storage.NewClient(
		ctx,
		option.WithHTTPClient(httpClient),
	)
}

func updateIndex(conf uploadChartsConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	storageClient, err := getGCSClient(ctx, conf.releaseConf.GCSURL)
	if err != nil {
		return fmt.Errorf("while creating gcs storage client: %w", err)
	}
	defer storageClient.Close()

	existingIndexFile := filepath.Join(conf.tempDirectory, "index.yaml.old")
	f, err := os.Create(existingIndexFile)
	if err != nil {
		return fmt.Errorf("while creating empty index.yaml: %w", err)
	}

	reader, err := storageClient.Bucket(conf.releaseConf.Bucket).Object("index.yaml").NewReader(ctx)
	if err != nil {
		return fmt.Errorf("while creating new reader for index.yaml: %w", err)
	}
	defer reader.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("while writing index.yaml.old: %w", err)
	}

	if err = f.Close(); err != nil {
		return fmt.Errorf("while closing index.yaml.old file: %w", err)
	}

	// helm repo index --merge index.yaml.old --url chart_repo_url temp_charts_location
	i, err := repo.IndexDirectory(conf.tempDirectory, conf.releaseConf.ChartsRepoURL)
	if err != nil {
		return fmt.Errorf("while indexing helm charts in temporary directory: %w", err)
	}

	i2, err := repo.LoadIndexFile(existingIndexFile)
	if err != nil {
		return fmt.Errorf("while loading existing helm index file: %w", err)
	}

	i.Merge(i2)
	i.SortEntries()

	if !conf.releaseConf.DryRun {
		log.Printf("Writing new helm index file for %s", conf.releaseConf.ChartsRepoURL)
		if err = i.WriteFile(filepath.Join(conf.tempDirectory, "index.yaml"), 0644); err != nil {
			return fmt.Errorf("while writing new helm index file: %w", err)
		}
		// Open new index.yaml
		f, err := os.Open(filepath.Join(conf.tempDirectory, "index.yaml"))
		if err != nil {
			return fmt.Errorf("while opening new index.yaml: %w", err)
		}
		defer f.Close()

		writer := storageClient.Bucket(conf.releaseConf.Bucket).Object("index.yaml").NewWriter(ctx)

		if _, err = io.Copy(writer, f); err != nil {
			return fmt.Errorf("while copying new index.yaml to bucket: %w", err)
		}

		if err := writer.Close(); err != nil {
			return fmt.Errorf("while writing new index.yaml to bucket: %w", err)
		}
	}
	return nil
}
