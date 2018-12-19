package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"

	"github.com/fsnotify/fsnotify"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log            = logf.Log.WithName("keystore-updater")
	sourceDir      string
	keystoreBinary string
	keystorePath   string
)

func fatal(err error, msg string) {
	log.Error(err, msg)
	os.Exit(1)
}

func updateKeystore() {
	// delete existing keystore (TODO can we do that to a running cluster?)
	_, err := os.Stat(keystorePath)
	if os.IsExist(err) {
		log.Info("Removing keystore", "keystore-path", keystorePath)
		err := os.Remove(keystorePath)
		if err != nil {
			fatal(err, "could not delete keystore file")
		}
	}

	log.Info("Creating keystore", "keystore-path", keystorePath)
	create := exec.Command(keystoreBinary, "create", "--silent")
	create.Dir = filepath.Dir(keystorePath)
	err = create.Run()
	if err != nil {
		fatal(err, "could not create new keystore")
	}

	fileInfos, err := ioutil.ReadDir(sourceDir)
	if err != nil {
		fatal(err, "could not read source directory")
	}
	for _, file := range fileInfos {
		log.Info("Adding setting to keystore", "file", file.Name())
		add := exec.Command(keystoreBinary, "add-file", file.Name(), path.Join(sourceDir, file.Name()))
		err := add.Run()
		if err != nil {
			fatal(err, fmt.Sprintf("could not add setting %s", file.Name()))
		}
	}

	list := exec.Command(keystoreBinary, "list")
	bytes, err := list.Output()
	if err != nil {
		fatal(err, "error during listing keystore settings")
	}

	re := regexp.MustCompile(`\r?\n`)
	input := re.ReplaceAllString(string(bytes), " ")
	log.Info("keystore updated", "settings", input)
}

func validateConfig() {
	_, err := os.Stat(sourceDir)
	if os.IsNotExist(err) {
		fatal(err, "source directory does not exist")
	}
	_, err = os.Stat(keystoreBinary)
	if os.IsNotExist(err) {
		fatal(err, "keystore binary does not exist")
	}
}

func main() {
	logf.SetLogger(logf.ZapLogger(false))

	flag.StringVar(&sourceDir, "source-dir", "/volumes/secrets", "directory containing keystore settings source files")
	flag.StringVar(&keystoreBinary, "keystore-binary", "/usr/share/elasticsearch/bin/elasticsearch-keystore", "path to keystore binary")
	flag.StringVar(&keystorePath, "keystore-path", "/usr/share/elasticsearch/config/elasticsearch.keystore", "path to keystore file")
	flag.Parse()

	validateConfig()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fatal(err, "Failed to create watcher")
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.Info("Observed:", "event", event)
				updateKeystore()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error(err, "watcher error")
			}
		}
	}()

	err = watcher.Add(sourceDir)
	if err != nil {
		fatal(err, fmt.Sprintf("failed to add watch on %s", sourceDir))
	}
	<-done

}
