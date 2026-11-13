// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pointer

// Int32OrDefault returns value pointed to by v, or def if it's nil
func Int32OrDefault(v *int32, def int32) int32 {
	if v == nil {
		return def
	}
	return *v
}
