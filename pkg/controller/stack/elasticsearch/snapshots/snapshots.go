package snapshots

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"

	"github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/pkg/errors"
)

const (
	// SnapshotRepositoryName is the name of the snapshot repository managed by this controller.
	SnapshotRepositoryName = "elastic-snapshots"
	// SnapshotClientName is the name of the Elasticsearch repository client.
	// TODO randomize name to avoid collisions with user created repos
	SnapshotClientName = "elastic-internal"
)

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
	// PhasePurge means we should start deleting snaphshots that are outside of the retention window.
	PhasePurge Phase = "purge"
)

func (s *Settings) nextPhase(snapshots []client.Snapshot, now time.Time) Phase {

	if len(snapshots) == 0 {
		return PhaseTake
	}
	latest := snapshots[len(snapshots)-1]
	if latest.IsInProgress() {
		// TODO  pending but stuck -> purge
		return PhaseWait
	}
	if latest.IsComplete() && latest.EndedBefore(s.Interval, time.Now()) {
		return PhaseTake
	}
	return PhasePurge
}

// snapshotsToPurge calculates the snapshots to delete based on the current settings.
// Invariant: snapshots should be sorted in descending order.
func (s *Settings) snapshotsToPurge(snapshots []client.Snapshot) []client.Snapshot {
	var toDelete []client.Snapshot
	successes := 0
	for _, snap := range snapshots {
		if successes < s.Max {
			if snap.IsSuccess() {
				successes++
			}
			//don't delete failures in the range of snapshots to keep
		} else {
			//we have kept the most recent n snapshots delete the rest
			toDelete = append(toDelete, snap)
		}
	}
	return toDelete
}

// purge deletes of the given snapshots. Invariant: descending order is assumed.
func (s *Settings) purge(esClient *client.Client, snapshots []client.Snapshot) error {
	// we delete only on snapshot at a time because we don't know what the underlying storage
	// mechanism of the snapshot repository is. In case of s3 we want to space operations to reach
	// consistency for example and avoid repository corruption.

	if len(snapshots) == 0 {
		return nil
	}
	toDelete := snapshots[len(snapshots)-1]
	//TODO how to keeep track of failed purges (k8s job?)
	return esClient.DeleteSnapshot(context.TODO(), s.Repository, toDelete.Snapshot)

}

func nextSnapshotName(now time.Time) string {
	return fmt.Sprintf("scheduled-%d", now.Unix())
}

// Maintain tries maintain the snapshot repository by either taking a new snapshot or if
// the most recent one is younger than the configured snapshot interval by trying to purge
// outdated snapshots.
func Maintain(esClient *client.Client, settings Settings) error {
	snapshotList, err := esClient.GetAllSnapshots(context.TODO(), settings.Repository)
	if err != nil {
		return err
	}
	snapshots := snapshotList.Snapshots
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].StartTime.After(snapshots[j].StartTime)
	})

	next := settings.nextPhase(snapshots, time.Now())
	switch next {
	case PhasePurge:
		return settings.purge(esClient, snapshots)
	case PhaseWait:
		return nil
	case PhaseTake:
		return esClient.TakeSnapshot(context.TODO(), settings.Repository, nextSnapshotName(time.Now()))

	}
	return nil
}

// RepositoryCredentialsKey returns a provider specific keystore key for the corresponding credentials.
func RepositoryCredentialsKey(repoConfig v1alpha1.SnapshotRepository) string {
	switch repoConfig.Type {
	case v1alpha1.SnapshotRepositoryTypeGCS:
		return common.Concat("gcs.client.", SnapshotClientName, ".credentials_file")
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
		return common.NewCompoundError(errs)

	default:
		return errors.New(common.Concat("Unsupported snapshot repository type ", string(kind)))
	}
}

// EnsureSnapshotRepository attempts to upsert a repository definition into the given cluster.
func EnsureSnapshotRepository(ctx context.Context, es *client.Client, repo v1alpha1.SnapshotRepository) error {
	expected := client.SnapshotRepository{
		Type: string(repo.Type),
		Settings: client.SnapshotRepositorySetttings{
			Bucket: repo.Settings.BucketName,
			Client: SnapshotClientName,
		},
	}
	current, err := es.GetSnapshotRepository(ctx, SnapshotRepositoryName)
	if err != nil && !client.IsNotFound(err) {
		return err
	}
	if current != expected {
		return es.UpsertSnapshotRepository(ctx, SnapshotRepositoryName, expected)
	}
	return nil

}
