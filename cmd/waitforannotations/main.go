// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package waitforannotations

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

const (
	FileFlag         = "file"
	AnnotationFlag   = "annotation"
	TimeoutFlag      = "timeout"
	PollIntervalFlag = "poll-interval"

	defaultPollInterval = 2 * time.Second
)

// Command returns the cobra.Command for the "wait-for-annotations" subcommand.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait-for-annotations",
		Short: "Block until all specified annotation keys are present in the downward-API annotations file",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
			if err := viper.BindPFlags(cmd.Flags()); err != nil {
				return fmt.Errorf("failed to bind flags: %w", err)
			}
			viper.AutomaticEnv()
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return doRun(signals.SetupSignalHandler())
		},
	}

	cmd.Flags().String(
		FileFlag,
		"",
		"Path to the downward-API annotations file",
	)
	cmd.Flags().StringArray(
		AnnotationFlag,
		nil,
		"Annotation key that must be present; may be specified multiple times",
	)
	cmd.Flags().Duration(
		TimeoutFlag,
		0,
		"Maximum time to wait; 0 means wait forever",
	)
	cmd.Flags().Duration(
		PollIntervalFlag,
		defaultPollInterval,
		"Interval between polls of the annotations file",
	)

	return cmd
}

// doRun implements the polling loop, reading configuration from viper.
func doRun(ctx context.Context) error {
	file := viper.GetString(FileFlag)
	if file == "" {
		return fmt.Errorf("--%s is required", FileFlag)
	}
	annotations := viper.GetStringSlice(AnnotationFlag)
	if len(annotations) == 0 {
		return fmt.Errorf("at least one --%s is required", AnnotationFlag)
	}
	timeout := viper.GetDuration(TimeoutFlag)
	pollInterval := viper.GetDuration(PollIntervalFlag)

	fmt.Printf("Waiting for annotations %v in %s\n", annotations, file)

	start := time.Now()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Check immediately before waiting for the first tick.
	for {
		if present, err := annotationsPresent(file, annotations); err != nil {
			// File unreadable / not yet created — not ready yet; continue polling.
			fmt.Printf("Annotations file not ready (%v), retrying...\n", err)
		} else if present {
			fmt.Printf("All expected annotations are set: %v\n", annotations)
			return nil
		}

		if timeout > 0 && time.Since(start) >= timeout {
			return fmt.Errorf("timed out after %s waiting for annotations %v in %s", timeout, annotations, file)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// annotationsPresent reports whether all required annotation keys are present in the
// downward-API annotations file at path. File format: one `key="value"` per line.
func annotationsPresent(path string, required []string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	found := make(map[string]struct{}, len(required))
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Extract the key: everything before the first '='.
		key, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		found[key] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}

	for _, key := range required {
		if _, ok := found[key]; !ok {
			return false, nil
		}
	}
	return true, nil
}
