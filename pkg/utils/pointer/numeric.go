// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pointer

// Int32 returns a pointer to an Int32
func Int32(v int32) *int32 { return &v }

// Int32OrDefault returns value pointed to by v, or def if it's nil
func Int32OrDefault(v *int32, def int32) int32 {
	if v == nil {
		return def
	}
	return *v
}

// Int64 returns a pointer to an Int64
func Int64(v int64) *int64 { return &v }
