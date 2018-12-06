package lvm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	log "github.com/sirupsen/logrus"
)

// RunLVMCmd runs the given LVM-related command,
// filters out known warnings from the output,
// and returns a JSON-unmarshalled input into result if given
func RunLVMCmd(cmd cmdutil.Executable, result interface{}) error {
	log.Infof("Running command: %v", cmd.Command())
	if err := cmd.Run(); err != nil {
		switch {
		case isInsufficientSpace(err):
			return ErrNoSpace
		case isInsufficientDevices(err):
			return ErrTooFewDisks
		case isLogicalVolumeNotFound(err):
			return ErrLogicalVolumeNotFound
		case isVolumeGroupNotFound(err):
			return ErrVolumeGroupNotFound
		default:
			return fmt.Errorf(ignoreWarnings(string(cmd.StdOut())))
		}
	}
	if result != nil {
		if err := json.Unmarshal(cmd.StdOut(), result); err != nil {
			return fmt.Errorf("cannot parse cmd output: %s %s", err.Error(), string(cmd.StdOut()))
		}
	}
	return nil
}

// ignoreWarnings ignores some lvm warnings we don't care about
func ignoreWarnings(str string) string {
	lines := strings.Split(str, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "WARNING") {
			log.Printf(line)
			continue
		}
		// Ignore warnings of the kind:
		// "File descriptor 13 (pipe:[120900]) leaked on vgs invocation."
		// For some reason lvm2 decided to complain if there are open file descriptors
		// that it didn't create when it exits.
		if strings.HasPrefix(line, "File descriptor") {
			log.Printf(line)
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}
