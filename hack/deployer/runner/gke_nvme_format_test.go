// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGKENvmeFormatScript(t *testing.T) {
	script := loadGKENvmeFormatScript(t)

	tests := []struct {
		name           string
		mountedSource  nvmeMountedSource
		filesystemType string
		wantError      bool
		wantOutput     string
		wantCommands   []string
	}{
		{
			name:          "reuses the expected mounted device",
			mountedSource: expectedDevice,
			wantOutput:    "already mounted at",
		},
		{
			name:          "rejects an unexpected mounted device",
			mountedSource: unexpectedDevice,
			wantError:     true,
			wantOutput:    "is mounted from unexpected source",
		},
		{
			name:         "formats and mounts a blank device",
			wantOutput:   "Formatting",
			wantCommands: []string{"mkfs.ext4", "mount"},
		},
		{
			name:           "mounts an existing ext4 filesystem without formatting",
			filesystemType: "ext4",
			wantOutput:     "already contains an ext4 filesystem",
			wantCommands:   []string{"mount"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newNVMeFormatterFixture(t, script)
			fixture.stubMountedSource(tt.mountedSource)
			fixture.filesystemType = tt.filesystemType

			result := fixture.run()

			if tt.wantError {
				require.Error(t, result.err)
			} else {
				require.NoError(t, result.err, result.output)
			}
			require.Contains(t, result.output, tt.wantOutput)
			require.Equal(t, tt.wantCommands, result.commands)
		})
	}
}

func loadGKENvmeFormatScript(t *testing.T) string {
	t.Helper()

	patchYAML, err := os.ReadFile("../config/local-disks-gke/gke-nvme-format-script-patch.yaml")
	require.NoError(t, err)

	var patches []struct {
		Value string `yaml:"value"`
	}
	require.NoError(t, yaml.Unmarshal(patchYAML, &patches))
	require.Len(t, patches, 1)
	return patches[0].Value
}

type nvmeFormatterFixture struct {
	t              *testing.T
	script         string
	device         string
	otherDevice    string
	mountPoint     string
	mountedSource  string
	filesystemType string
	commandLog     string
	binDir         string
}

type nvmeMountedSource int

const (
	noMountedDevice nvmeMountedSource = iota
	expectedDevice
	unexpectedDevice
)

func newNVMeFormatterFixture(t *testing.T, script string) *nvmeFormatterFixture {
	t.Helper()

	tempDir := t.TempDir()
	fixture := &nvmeFormatterFixture{
		t:           t,
		script:      script,
		device:      filepath.Join(tempDir, "expected-device"),
		otherDevice: filepath.Join(tempDir, "unexpected-device"),
		mountPoint:  filepath.Join(tempDir, "mount"),
		commandLog:  filepath.Join(tempDir, "commands.log"),
		binDir:      filepath.Join(tempDir, "bin"),
	}
	require.NoError(t, os.WriteFile(fixture.device, nil, 0o600))
	require.NoError(t, os.WriteFile(fixture.otherDevice, nil, 0o600))
	require.NoError(t, os.Mkdir(fixture.binDir, 0o700))
	fixture.installFakeCommands()
	return fixture
}

func (f *nvmeFormatterFixture) stubMountedSource(source nvmeMountedSource) {
	f.t.Helper()

	switch source {
	case noMountedDevice:
		f.mountedSource = ""
	case expectedDevice:
		f.mountedSource = f.device
	case unexpectedDevice:
		f.mountedSource = f.otherDevice
	default:
		f.t.Fatalf("unknown mounted source %d", source)
	}
}

func (f *nvmeFormatterFixture) installFakeCommands() {
	f.t.Helper()

	writeExecutable(f.t, filepath.Join(f.binDir, "mountpoint"), `#!/bin/sh
[ -n "${MOUNTED_SOURCE}" ]
`)
	writeExecutable(f.t, filepath.Join(f.binDir, "findmnt"), `#!/bin/sh
case "$*" in
  *"-o SOURCE"*)
    [ -n "${MOUNTED_SOURCE}" ] || exit 1
    printf '%s\n' "${MOUNTED_SOURCE}"
    exit 0
    ;;
esac
exit 1
`)
	writeExecutable(f.t, filepath.Join(f.binDir, "readlink"), "#!/bin/sh\nprintf '%s\\n' \"$2\"\n")
	writeExecutable(f.t, filepath.Join(f.binDir, "blkid"), `#!/bin/sh
if [ -n "${FILESYSTEM_TYPE}" ]; then
  printf '%s\n' "${FILESYSTEM_TYPE}"
  exit 0
fi
exit 2
`)
	writeExecutable(f.t, filepath.Join(f.binDir, "mkfs.ext4"), "#!/bin/sh\necho mkfs.ext4 >> \"${COMMAND_LOG}\"\n")
	writeExecutable(f.t, filepath.Join(f.binDir, "mount"), "#!/bin/sh\necho mount >> \"${COMMAND_LOG}\"\n")
}

type nvmeFormatterResult struct {
	output   string
	commands []string
	err      error
}

func (f *nvmeFormatterFixture) run() nvmeFormatterResult {
	f.t.Helper()

	scriptPath := filepath.Join(f.t.TempDir(), "gke-nvme-format.sh")
	writeExecutable(f.t, scriptPath, f.script)

	cmd := exec.CommandContext(f.t.Context(), "sh", scriptPath)
	cmd.Env = append(withoutPath(os.Environ()),
		"PATH="+f.binDir+":"+os.Getenv("PATH"),
		"GKE_LOCAL_SSD_DEVICE="+f.device,
		"GKE_LOCAL_SSD_MOUNT_POINT="+f.mountPoint,
		"MOUNTED_SOURCE="+f.mountedSource,
		"FILESYSTEM_TYPE="+f.filesystemType,
		"COMMAND_LOG="+f.commandLog,
	)
	output, commandErr := cmd.CombinedOutput()

	commandLogContents, err := os.ReadFile(f.commandLog)
	if err != nil && !os.IsNotExist(err) {
		require.NoError(f.t, err)
	}
	var commands []string
	if err == nil {
		commands = strings.Fields(string(commandLogContents))
	}
	return nvmeFormatterResult{
		output:   string(output),
		commands: commands,
		err:      commandErr,
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o700))
}

func withoutPath(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, value := range environment {
		if !strings.HasPrefix(value, "PATH=") {
			result = append(result, value)
		}
	}
	return result
}
