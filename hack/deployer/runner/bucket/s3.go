// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bucket

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/hack/deployer/exec"
)

// S3Manager manages Amazon S3 buckets.
type S3Manager struct {
	cfg Config
	s3  S3Config
}

var _ Manager = &S3Manager{}

// NewS3Manager creates a new S3 bucket manager.
// The S3-specific configuration (IAM path, managed policy ARN) is passed separately
// from the common Config to make the dependency explicit.
func NewS3Manager(cfg Config, s3 S3Config) *S3Manager {
	return &S3Manager{
		cfg: cfg,
		s3:  s3,
	}
}

func (s *S3Manager) iamUserName() string {
	// IAM user names can be up to 64 chars. Reserve space for the prefix and suffix
	// so that the "-storage" suffix is always preserved — deleteIAMUser relies on it
	// as an ownership check before deletion.
	const prefix, suffix = "eck-bkt-", "-storage"
	bucketName := s.cfg.Name
	if maxBucketLen := 64 - len(prefix) - len(suffix); len(bucketName) > maxBucketLen {
		bucketName = bucketName[:maxBucketLen]
	}
	return prefix + bucketName + suffix
}

// Create creates an S3 bucket, a scoped IAM user with access keys, and a Kubernetes Secret.
func (s *S3Manager) Create() error {
	if err := s.createBucket(); err != nil {
		return err
	}
	if err := s.tagBucket(); err != nil {
		return err
	}
	if err := s.blockPublicAccess(); err != nil {
		return err
	}
	accessKeyID, secretAccessKey, err := s.createIAMUserAndKeys()
	if err != nil {
		return err
	}

	return createK8sSecret(s.cfg.SecretName, s.cfg.SecretNamespace, map[string]string{
		"access-key-id":     accessKeyID,
		"secret-access-key": secretAccessKey,
		"bucket":            s.cfg.Name,
		"region":            s.cfg.Region,
	})
}

// Delete removes the S3 bucket, its contents, and the associated IAM user.
// Both deletions are attempted even if one fails to avoid leaking cloud resources.
// Each sub-function verifies ownership before deleting (IAM path and naming convention for the user, managed_by tag for the bucket).
func (s *S3Manager) Delete() error {
	bucketErr := s.deleteBucket()
	iamErr := s.deleteIAMUser()
	return errors.Join(bucketErr, iamErr)
}

func (s *S3Manager) createBucket() error {
	log.Printf("Creating S3 bucket %s in region %s...", s.cfg.Name, s.cfg.Region)

	// Check if bucket already exists
	checkCmd := fmt.Sprintf("aws s3api head-bucket --bucket %s --region %s", s.cfg.Name, s.cfg.Region)
	output, err := exec.NewCommand(checkCmd).WithoutStreaming().Output()
	if err == nil {
		log.Printf("Bucket %s already exists, skipping creation", s.cfg.Name)
		return nil
	}
	// head-bucket returns 404 for non-existent buckets; any other error (auth, network) should surface.
	if !isNotFound(output, "404", "Not Found", "NoSuchBucket") {
		return fmt.Errorf("while checking if bucket %s exists: %w", s.cfg.Name, err)
	}

	// us-east-1 does not accept LocationConstraint
	if s.cfg.Region == "us-east-1" {
		cmd := fmt.Sprintf("aws s3api create-bucket --bucket %s --region %s", s.cfg.Name, s.cfg.Region)
		return exec.NewCommand(cmd).Run()
	}

	cmd := fmt.Sprintf(
		"aws s3api create-bucket --bucket %s --region %s --create-bucket-configuration LocationConstraint=%s",
		s.cfg.Name, s.cfg.Region, s.cfg.Region,
	)
	return exec.NewCommand(cmd).Run()
}

// blockPublicAccess ensures S3 Block Public Access is enabled on the bucket.
// This is the default for new buckets since April 2023, but we set it explicitly
// to guard against account-level policy overrides.
func (s *S3Manager) blockPublicAccess() error {
	log.Printf("Ensuring S3 Block Public Access is enabled on bucket %s...", s.cfg.Name)
	cmd := fmt.Sprintf(
		"aws s3api put-public-access-block --bucket %s --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true",
		s.cfg.Name,
	)
	return exec.NewCommand(cmd).Run()
}

// tagBucket applies resource tags to the S3 bucket. This is a separate call because
// aws s3api create-bucket does not support tags at creation time.
func (s *S3Manager) tagBucket() error {
	log.Printf("Tagging S3 bucket %s...", s.cfg.Name)

	type tag struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	}
	type tagging struct {
		TagSet []tag `json:"TagSet"`
	}

	t := tagging{}
	for k, v := range s.cfg.Labels {
		t.TagSet = append(t.TagSet, tag{Key: k, Value: v})
	}
	taggingJSON, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("while marshalling bucket tags: %w", err)
	}

	cmd := fmt.Sprintf(
		`aws s3api put-bucket-tagging --bucket %s --tagging '%s'`,
		s.cfg.Name, string(taggingJSON),
	)
	return exec.NewCommand(cmd).Run()
}

// accessKeyOutput matches the AWS CLI output for create-access-key.
type accessKeyOutput struct {
	AccessKey struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
	} `json:"AccessKey"`
}

func (s *S3Manager) createIAMUserAndKeys() (string, string, error) {
	userName := s.iamUserName()

	log.Printf("Creating IAM user %s under path %s...", userName, s.s3.IAMUserPath)

	// Create IAM user
	createCmd := fmt.Sprintf("aws iam create-user --user-name %s --path %s", userName, s.s3.IAMUserPath)
	if err := exec.NewCommand(createCmd).WithoutStreaming().Run(); err != nil {
		// Check if user already exists
		checkCmd := fmt.Sprintf("aws iam get-user --user-name %s", userName)
		if checkErr := exec.NewCommand(checkCmd).WithoutStreaming().Run(); checkErr != nil {
			return "", "", fmt.Errorf("while creating IAM user: %w", err)
		}
		log.Printf("IAM user %s already exists", userName)
	}

	// Attach the pre-existing managed policy that grants S3 access to buckets
	// ending with -snapshot-repo, -logs, or -development.
	log.Printf("Attaching managed policy %s to IAM user %s...", s.s3.ManagedPolicyARN, userName)
	attachCmd := fmt.Sprintf(
		"aws iam attach-user-policy --user-name %s --policy-arn %s",
		userName, s.s3.ManagedPolicyARN,
	)
	if err := exec.NewCommand(attachCmd).WithoutStreaming().Run(); err != nil {
		return "", "", fmt.Errorf("while attaching managed policy to IAM user: %w", err)
	}

	// Create access key
	log.Printf("Creating access key for IAM user %s...", userName)
	keyCmd := fmt.Sprintf("aws iam create-access-key --user-name %s", userName)
	output, err := exec.NewCommand(keyCmd).WithoutStreaming().Output()
	if err != nil {
		return "", "", fmt.Errorf("while creating access key: %w", err)
	}

	var keyOutput accessKeyOutput
	if err := json.Unmarshal([]byte(output), &keyOutput); err != nil {
		return "", "", fmt.Errorf("while parsing access key output: %w", err)
	}

	return keyOutput.AccessKey.AccessKeyID, keyOutput.AccessKey.SecretAccessKey, nil
}

// iamGetUserOutput matches the AWS CLI output for iam get-user.
type iamGetUserOutput struct {
	User struct {
		Path     string `json:"Path"`
		UserName string `json:"UserName"`
	} `json:"User"`
}

func (s *S3Manager) deleteIAMUser() error {
	userName := s.iamUserName()
	log.Printf("Verifying IAM user %s is managed by eck-deployer...", userName)

	// Verify via path and naming convention instead of tags
	// (the IAM management policy may not grant iam:TagUser or iam:ListUserTags).
	getUserCmd := fmt.Sprintf("aws iam get-user --user-name %s", userName)
	output, err := exec.NewCommand(getUserCmd).WithoutStreaming().Output()
	if err != nil {
		if isNotFound(output, "NoSuchEntity") {
			log.Printf("IAM user %s not found, skipping deletion", userName)
			return nil
		}
		return fmt.Errorf("while checking IAM user %s: %w", userName, err)
	}

	var getUserOutput iamGetUserOutput
	if err := json.Unmarshal([]byte(output), &getUserOutput); err != nil {
		return fmt.Errorf("while parsing IAM user %s: %w", userName, err)
	}
	if getUserOutput.User.Path != s.s3.IAMUserPath {
		return fmt.Errorf(
			"refusing to delete IAM user %s: expected path %s but found %q. Delete it manually",
			userName, s.s3.IAMUserPath, getUserOutput.User.Path,
		)
	}
	if !strings.HasSuffix(getUserOutput.User.UserName, "-storage") {
		return fmt.Errorf(
			"refusing to delete IAM user %s: username does not end with -storage. Delete it manually",
			userName,
		)
	}

	log.Printf("Deleting IAM user %s and associated resources...", userName)

	// List and delete access keys — must succeed before the user can be deleted.
	listKeysCmd := fmt.Sprintf(`aws iam list-access-keys --user-name %s --query "AccessKeyMetadata[].AccessKeyId" --output text`, userName)
	keysOutput, err := exec.NewCommand(listKeysCmd).WithoutStreaming().Output()
	if err != nil {
		return fmt.Errorf("while listing access keys for IAM user %s: %w", userName, err)
	}
	for keyID := range strings.FieldsSeq(strings.TrimSpace(keysOutput)) {
		delKeyCmd := fmt.Sprintf("aws iam delete-access-key --user-name %s --access-key-id %s", userName, keyID)
		if err := exec.NewCommand(delKeyCmd).WithoutStreaming().Run(); err != nil {
			return fmt.Errorf("while deleting access key %s for IAM user %s: %w", keyID, userName, err)
		}
	}

	// Detach managed policy — must succeed before the user can be deleted.
	detachCmd := fmt.Sprintf("aws iam detach-user-policy --user-name %s --policy-arn %s", userName, s.s3.ManagedPolicyARN)
	if err := exec.NewCommand(detachCmd).WithoutStreaming().Run(); err != nil {
		return fmt.Errorf("while detaching policy from IAM user %s: %w", userName, err)
	}

	// Delete the user
	delUserCmd := fmt.Sprintf("aws iam delete-user --user-name %s", userName)
	if err := exec.NewCommand(delUserCmd).WithoutStreaming().Run(); err != nil {
		return fmt.Errorf("while deleting IAM user %s: %w", userName, err)
	}
	return nil
}

func (s *S3Manager) deleteBucket() error {
	log.Printf("Verifying S3 bucket %s is managed by eck-deployer...", s.cfg.Name)
	tagCmd := fmt.Sprintf(
		`aws s3api get-bucket-tagging --bucket %s --region %s --query "TagSet[?Key=='%s'].Value" --output text`,
		s.cfg.Name, s.cfg.Region, ManagedByTag,
	)
	output, err := exec.NewCommand(tagCmd).WithoutStreaming().Output()
	if err != nil {
		if isNotFound(output, "NoSuchBucket") {
			log.Printf("Bucket %s not found, skipping deletion", s.cfg.Name)
			return nil
		}
		// NoSuchTagSet means the bucket exists but has no tags — likely the tagging step
		// failed during creation. Surface this as an error so the bucket is not silently orphaned.
		if isNotFound(output, "NoSuchTagSet") {
			return fmt.Errorf(
				"bucket %s exists but has no tags (tagging may have failed during creation); delete it manually or re-tag it with %s=%s",
				s.cfg.Name, ManagedByTag, ManagedByValue,
			)
		}
		return fmt.Errorf("while checking S3 bucket %s tags: %w", s.cfg.Name, err)
	}
	value := strings.TrimSpace(output)
	if value != ManagedByValue {
		return fmt.Errorf(
			"refusing to delete S3 bucket %s: missing tag %s=%s (found %q). Delete it manually",
			s.cfg.Name, ManagedByTag, ManagedByValue, value,
		)
	}

	log.Printf("Deleting S3 bucket %s...", s.cfg.Name)
	// Delete all objects first
	rmCmd := fmt.Sprintf("aws s3 rm s3://%s --recursive --region %s", s.cfg.Name, s.cfg.Region)
	if err := exec.NewCommand(rmCmd).Run(); err != nil {
		log.Printf("warning: failed to remove objects from bucket %s: %v", s.cfg.Name, err)
	}
	// Delete the bucket
	delCmd := fmt.Sprintf("aws s3api delete-bucket --bucket %s --region %s", s.cfg.Name, s.cfg.Region)
	if err := exec.NewCommand(delCmd).Run(); err != nil {
		return fmt.Errorf("while deleting bucket %s: %w", s.cfg.Name, err)
	}
	return nil
}
