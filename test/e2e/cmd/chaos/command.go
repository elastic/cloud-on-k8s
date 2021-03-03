// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package chaos

import (
	"time"

	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	log = ulog.Log.WithName("chaos")

	// defaultDeleteOperatorPodDelay is the default delay between deletions. It is an arbitrary value which
	// avoid to delete too often an operator and therefore inadvertently unlock some situations (e.g. election
	// deadlocks) we may want to observe. In the meantime, assuming that e2e session lasts a couple of hours,
	// it allows to observe quite a few times the behaviour of the operator when its Pods are deleted.
	defaultDeleteOperatorPodDelay            = 9 * time.Minute
	defaultChangeOperatorReplicasDelay       = 30 * time.Minute
	minReplicas                        int32 = 1
	maxReplicas                        int32 = 3

	// checkLeaderDelay defines how often we attempt to check if there is at most one elected operator.
	checkLeaderDelay = 5 * time.Second
)

type runFlags struct {
	logVerbosity       int
	autoPortForwarding bool
	operatorNamespace  string
	operatorName       string

	// deleteOperatorPodDelay defines how often a random operator Pod is deleted.
	// Operator Pods should not been deleted too often as deleting a Pod may resolve a deadlock issue we want to detect.
	deleteOperatorPodDelay time.Duration

	// changeOperatorReplicasDelay defines how often the number of replicas is changed from minReplicas to maxReplicas
	// or the other way round.
	changeOperatorReplicasDelay time.Duration
}

func Command() *cobra.Command {
	flags := runFlags{}

	cmd := &cobra.Command{
		Use:   "chaos",
		Short: "randomly delete operator Pod",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags.logVerbosity, _ = cmd.PersistentFlags().GetInt("log-verbosity")
			err := doRun(flags)
			if err != nil {
				log.Error(err, "Failed to run chaos process")
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&flags.autoPortForwarding, "auto-port-forwarding", false, "Enable port forwarding to pods")
	cmd.Flags().StringVar(&flags.operatorNamespace, "operator-namespace", "", "Namespace in which the operator Pods are deployed")
	cmd.Flags().StringVar(&flags.operatorName, "operator-name", "", "Operator name as it appears in the control-plane label")

	cmd.Flags().DurationVar(&flags.deleteOperatorPodDelay, "delete-operator-delay", defaultDeleteOperatorPodDelay, "Delay between two operator Pod deletions")
	cmd.Flags().DurationVar(&flags.changeOperatorReplicasDelay, "update-operator-replicas-delay", defaultChangeOperatorReplicasDelay, "Delay between two operator replicas updates")
	ulog.BindFlags(cmd.PersistentFlags())

	// enable setting flags via environment variables
	_ = viper.BindPFlags(cmd.Flags())

	return cmd
}
