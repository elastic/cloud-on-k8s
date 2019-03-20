// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	// SnapshotRepositoryName is the name of the snapshot repository managed by this controller.
	SnapshotRepositoryName = "elastic-snapshots"
	// SnapshotClientName is the name of the Elasticsearch repository client.
	// TODO randomize name to avoid collisions with user created repos
	SnapshotClientName = "elastic-internal"
)

var (
	log = logf.Log.WithName("snapshots")
)

// SnapshotAPI contains Elasticsearch API calls related to snapshots.
type SnapshotAPI interface {
	// GetAllSnapshots returns a list of all snapshots for the given repository.
	GetAllSnapshots(ctx context.Context, repo string) (client.SnapshotsList, error)
	// TakeSnapshot takes a new cluster snapshot with the given name into the given repository.
	TakeSnapshot(ctx context.Context, repo string, snapshot string) error
	// DeleteSnapshot delete the given snapshot from the given repository.
	DeleteSnapshot(ctx context.Context, repo string, snapshot string) error
}

// Settings define the how often, how long to keep and where to for snapshotting.
type Settings struct {
	Interval   time.Duration
	Max        int
	Repository string
}

// Phase is the one of the three phases the snapshot job can be in.
type Phase string

const (
	// PhaseWait means a snapshot is still running and we have to wait.
	PhaseWait Phase = "wait"
	// PhaseTake means a snapshot should be taken now.
	PhaseTake Phase = "take"
	// PhasePurge means we should start deleting snapshots that are outside of the retention window.
	PhasePurge Phase = "purge"
)

func (s *Settings) nextPhase(snapshots []client.Snapshot, now time.Time) Phase {
	if len(snapshots) == 0 {
		return PhaseTake
	}
	latest := snapshots[0]
	log.Info(fmt.Sprintf("Latest snapshot is: [%s] state: [%s] started: [%s] ended: [%s]", latest.Snapshot, latest.State, latest.StartTime, latest.EndTime))
	if latest.IsInProgress() {
		// TODO  pending but stuck -> purge
		return PhaseWait
	}
	if latest.IsComplete() && latest.EndedBefore(s.Interval, now) {
		return PhaseTake
	}
	return PhasePurge
}

// snapshotsToPurge calculates the snapshots to delete based on the current settings.
// Invariant: snapshots should be sorted in descending order.
func snapshotsToPurge(snapshots []client.Snapshot, settings Settings) []client.Snapshot {
	var toPurge []client.Snapshot
	successes := 0
	for _, snap := range snapshots {
		if successes < settings.Max {
			if snap.IsSuccess() {
				successes++
			}
			// failures don't count towards the retention limit
		} else {
			//we have kept the most recent n snapshots delete the rest
			toPurge = append(toPurge, snap)
		}
	}
	log.Info(fmt.Sprintf("With max snapshots being %d found %d to delete", settings.Max, len(toPurge)))
	return toPurge
}

// purge deletes the oldest of the given snapshots. Invariant: descending order is assumed.
func (s *Settings) purge(esClient SnapshotAPI, snapshots []client.Snapshot) error {
	// we delete only one snapshot at a time because we don't know what the underlying storage
	// mechanism of the snapshot repository is. In case of s3 we want to space operations to reach
	// consistency for example and avoid repository corruption.
	if len(snapshots) == 0 {
		return nil
	}
	toDelete := snapshots[len(snapshots)-1]
	log.Info(fmt.Sprintf("Deleting snapshot [%s]", toDelete.Snapshot))
	//TODO how to keeep track of failed purges?
	return esClient.DeleteSnapshot(context.TODO(), s.Repository, toDelete.Snapshot)
}

func nextSnapshotName(now time.Time) string {
	return fmt.Sprintf("scheduled-%d", now.Unix())
}

// ExecuteNextPhase tries to maintain the snapshot repository by either taking a new snapshot or if
// the most recent one is younger than the configured snapshot interval by trying to purge
// outdated snapshots.
func ExecuteNextPhase(esClient SnapshotAPI, settings Settings) error {
	snapshotList, err := esClient.GetAllSnapshots(context.TODO(), settings.Repository)
	if err != nil {
		return err
	}
	snapshots := snapshotList.Snapshots
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].StartTime.After(snapshots[j].StartTime)
	})

	log.Info(fmt.Sprintf("Getting all snapshots. Found [%d] snapshots", (len(snapshots))))
	next := settings.nextPhase(snapshots, time.Now())
	log.Info(fmt.Sprintf("Operation is [%s]", string(next)))
	switch next {
	case PhasePurge:
		return settings.purge(esClient, snapshotsToPurge(snapshots, settings))
	case PhaseWait:
		return nil
	case PhaseTake:
		return esClient.TakeSnapshot(context.TODO(), settings.Repository, nextSnapshotName(time.Now()))
	}
	return nil
}

// RepositoryCredentialsKey returns a provider specific keystore key for the corresponding credentials.
func RepositoryCredentialsKey(repoConfig *v1alpha1.SnapshotRepository) string {
	switch repoConfig.Type {
	case v1alpha1.SnapshotRepositoryTypeGCS:
		return stringsutil.Concat("gcs.client.", SnapshotClientName, ".credentials_file")
	}
	return ""
}

func validateGcsKeyFile(fileName string, credentials map[string]string) error {
	expected := []string{
		"type",
		"project_id",
		"private_key_id",
		"private_key",
		"client_email",
		"client_id",
		"auth_uri",
		"token_uri",
		"auth_provider_x509_cert_url",
		"client_x509_cert_url",
	}
	var missing []string
	var err error
	for _, k := range expected {
		if _, ok := credentials[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		err = errors.Errorf("Expected keys %v not found in %s gcs credential file", missing, fileName)
	}
	return err

}

// ValidateSnapshotCredentials does a superficial inspection of provider specific snapshot credentials typically retrieved via a secret.
func ValidateSnapshotCredentials(kind v1alpha1.SnapshotRepositoryType, raw map[string][]byte) error {
	switch kind {
	case v1alpha1.SnapshotRepositoryTypeGCS:
		var errs []error
		for k, v := range raw {
			var parsed map[string]string
			err := json.Unmarshal(v, &parsed)
			if err != nil {
				errs = append(errs, errors.Wrap(err, "gcs secrets need to be JSON, PKCS12 is not supported"))
				continue
			}
			if err := validateGcsKeyFile(k, parsed); err != nil {
				errs = append(errs, err)
			}
		}
		return k8serrors.NewAggregate(errs)

	default:
		return errors.New(stringsutil.Concat("Unsupported snapshot repository type ", string(kind)))
	}
}

// ReconcileSnapshotRepository attempts to upsert a repository definition into the given cluster.
func ReconcileSnapshotRepository(ctx context.Context, es client.Client, repo *v1alpha1.SnapshotRepository) error {

	current, err := es.GetSnapshotRepository(ctx, SnapshotRepositoryName)
	if err != nil && !client.IsNotFound(err) {
		return err
	}

	if repo == nil {
		if err == nil { // we have a repository in ES delete it
			log.Info("Deleting existing snapshot repository")
			return es.DeleteSnapshotRepository(ctx, SnapshotRepositoryName)
		}
		return nil // we don't have one and we don't want one
	}

	expected := client.SnapshotRepository{
		Type: string(repo.Type),
		Settings: client.SnapshotRepositorySetttings{
			Bucket: repo.Settings.BucketName,
			Client: SnapshotClientName,
		},
	}

	if current != expected {
		log.Info("Updating snapshot repository")
		return es.UpsertSnapshotRepository(ctx, SnapshotRepositoryName, expected)
	}
	return nil

}
