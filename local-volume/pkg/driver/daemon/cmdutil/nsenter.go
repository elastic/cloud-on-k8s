package cmdutil

import (
	"fmt"
	"os/exec"
)

// HostProcNsPath is the path to directory /proc/1/ns directory
// mounted from the host
const HostProcNsPath = "/hostprocns"
const nsEnterCmd = "nsenter"

var nsEnterArgs = []string{fmt.Sprintf("--mount=%s/mnt", HostProcNsPath), "--"}

// NSEnterWrap wraps the given command with nsenter,
// to use the host mount namespace
func NSEnterWrap(cmd ...string) *exec.Cmd {
	return exec.Command(nsEnterCmd, append(nsEnterArgs, cmd...)...)
}
