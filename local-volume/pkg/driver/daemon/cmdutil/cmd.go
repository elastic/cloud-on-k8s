package cmdutil

import (
	"fmt"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

// RunCmd runs the given command, and returns a combined output
// of the err message, stdout and stderr in case of error
func RunCmd(cmd *exec.Cmd) error {
	log.Infof("Running command: %v", cmd.Args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s. Output: %s", err.Error(), string(output[:]))
	}
	return nil
}
