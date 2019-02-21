package fs

import (
	gopath "path"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

// WatchPath watches changes on the given path, and calls onEvent when something happens.
// It returns when onEvent returns true or an error. Otherwise, it runs forever.
// Changes on file names starting with a dot are ignored.
func WatchPath(path string, onEvent func() (stop bool, err error), logger logr.Logger) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	if err := watcher.Add(path); err != nil {
		return err
	}
	// an update might have occured just before the watcher setup
	if stop, err := onEvent(); err != nil || stop {
		return err
	}
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Chmod == fsnotify.Chmod || strings.HasPrefix(gopath.Base(event.Name), ".") {
				// avoid noisy chmod events when k8s maps changes into the file system
				// also k8s seems to use a couple of dot files to manage mapped secrets which create
				// additional noise and should be safe to ignore
				continue
			}
			logger.Info("Event observed", "event", event)
			if stop, err := onEvent(); err != nil || stop {
				return err
			}

		case err := <-watcher.Errors:
			logger.Error(err, "watcher error")
		}
	}
}
