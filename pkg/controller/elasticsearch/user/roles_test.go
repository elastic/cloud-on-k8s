// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
)

func Test_RolesFileContent(t *testing.T) {
	rolesBytes := []byte(`another_role:
  cluster: [ 'all' ]
click_admins:
  run_as: [ 'clicks_watcher_1' ]
  cluster: [ 'monitor' ]
  indices:
  - names: [ 'events-*' ]
    privileges: [ 'read' ]
    field_security:
      grant: ['category', '@timestamp', 'message' ]
    query: '{"match": {"category": "click"}}'`)

	// unmarshal into rolesFileContent
	var r RolesFileContent
	require.NoError(t, yaml.Unmarshal(rolesBytes, &r))
	// should be a map[string]interface{} whose keys are the role names
	require.Len(t, r, 2)
	require.NotEmpty(t, r["click_admins"])
	require.NotEmpty(t, r["another_role"])
	require.Empty(t, r["unknown_role"])
	// .fileBytes() should unmarshal back to the original yaml but may be formatted differently (indent, lists, etc.)
	// so we don't compare the raw yaml explicitly here
	asBytes, err := r.FileBytes()
	require.NoError(t, err)
	// instead, compare the internal representation
	// unmarshal yaml back into rolesFileContent again
	var rAgain RolesFileContent
	require.NoError(t, yaml.Unmarshal(asBytes, &rAgain))
	// the initial yaml and the serialized/de-serialized rolesFileContent should be the same
	require.Equal(t, r, rAgain)
}
