// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type MockCmd struct {
	cmd    *exec.Cmd
	output []byte
}

func (m *MockCmd) Run(cmd *exec.Cmd) error {
	// keep a reference to cmd for inspection
	m.cmd = cmd
	return nil
}

func (m *MockCmd) Output(cmd *exec.Cmd) ([]byte, error) {
	// keep a reference to cmd for inspection
	m.cmd = cmd
	return m.output, nil
}

func Test_keystore_AddSetting(t *testing.T) {
	// work with settings in a tmp dir
	tmpDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// build a keystore with mocked commands
	mockCmd := &MockCmd{}
	keystore := keystore{
		keystorePath: "/usr/share/elasticsearch/config/elasticsearch.keystore",
		binaryPath:   "/usr/share/elasticsearch/bin/elasticsearch-keystore",
		settingsPath: tmpDir,
		cmdRunner:    mockCmd,
	}

	tests := []struct {
		name          string
		filename      string
		fileContent   []byte
		expectedCmd   string
		expectedStdin []byte
	}{
		{
			name:          "string setting",
			filename:      "es.string.my.setting",
			fileContent:   []byte("setting value"),
			expectedCmd:   "/usr/share/elasticsearch/bin/elasticsearch-keystore add my.setting",
			expectedStdin: []byte("setting value"),
		},
		{
			name:          "file setting",
			filename:      "es.file.my.setting",
			fileContent:   []byte("setting value"),
			expectedCmd:   "/usr/share/elasticsearch/bin/elasticsearch-keystore add-file my.setting " + filepath.Join(tmpDir, "es.file.my.setting"),
			expectedStdin: nil,
		},
		{
			name:          "default to string setting if no prefix provided",
			filename:      "my.setting",
			fileContent:   []byte("setting value"),
			expectedCmd:   "/usr/share/elasticsearch/bin/elasticsearch-keystore add my.setting",
			expectedStdin: []byte("setting value"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// write the setting file
			err = ioutil.WriteFile(filepath.Join(tmpDir, tt.filename), tt.fileContent, 0644)
			require.NoError(t, err)

			// add it to the keystore
			err = keystore.AddSetting(tt.filename)
			require.NoError(t, err)

			// verify the right command was called
			require.Equal(t, strings.Split(tt.expectedCmd, " "), mockCmd.cmd.Args)

			// verify the right content was piped in
			if tt.expectedStdin != nil {
				stdin := make([]byte, len(tt.fileContent))
				_, err = mockCmd.cmd.Stdin.Read(stdin)
				require.NoError(t, err)
				require.Equal(t, tt.fileContent, stdin)
			}
		})
	}
}
