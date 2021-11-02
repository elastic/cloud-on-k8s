// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package run

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const newLine = byte('\n')

// jsonLog implements a Writer that only emits valid JSON to output.
type jsonLog struct {
	out    io.WriteCloser
	writer *bufio.Writer
	buf    *bytes.Buffer
}

func newJSONLogToFile(outFile string) (*jsonLog, error) {
	f, err := os.Create(outFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON log file %s: %w", outFile, err)
	}

	return newJSONLog(f), nil
}

func newJSONLog(out io.WriteCloser) *jsonLog {
	return &jsonLog{
		out:    out,
		writer: bufio.NewWriter(out),
		buf:    new(bytes.Buffer),
	}
}

func (jl *jsonLog) Write(p []byte) (int, error) {
	for i, b := range p {
		if b == newLine {
			if err := jl.writeLine(); err != nil {
				return 0, err
			}
			continue
		}
		if err := jl.buf.WriteByte(b); err != nil {
			return i, err
		}
	}
	if err := jl.flush(); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (jl *jsonLog) writeLine() error {
	line := jl.buf.Bytes()
	if json.Valid(line) {
		if _, err := jl.writer.Write(line); err != nil {
			return err
		}
		if _, err := jl.writer.Write([]byte{newLine}); err != nil {
			return err
		}
	}

	jl.buf.Reset()
	return nil
}

func (jl *jsonLog) Close() (err error) {
	defer func() {
		e := jl.out.Close()
		if err == nil {
			err = e
		}
	}()

	if err = jl.flush(); err != nil {
		return
	}

	return nil
}

func (jl *jsonLog) flush() (err error) {
	// flush internal buffer
	if err = jl.writeLine(); err != nil {
		return
	}

	// flush the writer
	if err = jl.writer.Flush(); err != nil {
		return
	}
	return nil
}
