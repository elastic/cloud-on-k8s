// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package optional

import (
	"bytes"
	"encoding/json"
	"fmt"
)

var null = []byte("null")

type Bool struct {
	value bool
}

// UnmarshalJSON implements json.Unmarshaler.
// It supports number and null input.
// 0 will not be considered a null Bool.
func (b *Bool) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, null) {
		return nil
	}
	if err := json.Unmarshal(data, &b.value); err != nil {
		return fmt.Errorf("null: couldn't unmarshal JSON: %w", err)
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
// It will encode null if this Bool is null.
func (b *Bool) MarshalJSON() ([]byte, error) {
	if b == nil {
		return null, nil
	}
	if !b.value {
		return []byte("false"), nil
	}
	return []byte("true"), nil
}

func (b *Bool) IsSet() bool {
	return b != nil
}

func (b *Bool) IsTrue() bool {
	return b != nil && b.value
}

func (b *Bool) IsFalse() bool {
	return b != nil && !b.value
}

func NewBool(value bool) *Bool {
	return &Bool{value: value}
}

func (b *Bool) Or(other *Bool) *Bool {
	if b != nil && other != nil {
		return NewBool(b.value || other.value)
	}
	if b == nil {
		return other
	}
	if other == nil {
		return b
	}
	return nil
}
