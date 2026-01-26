package backup

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	fc_config "github.com/figchain/go-client/pkg/config"
)

// BackupFetcher defines the interface for fetching backup files.
type BackupFetcher interface {
	FetchBackup(ctx context.Context, keyFingerprint string) (io.ReadCloser, error)
}

// S3BackupFetcher fetches backup files from S3.
type S3BackupFetcher struct {
	client     *s3.Client
	bucketName string
	prefix     string
}

// NewS3BackupFetcher creates a new S3BackupFetcher.
func NewS3BackupFetcher(ctx context.Context, cfg *fc_config.Config) (*S3BackupFetcher, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	if cfg.S3BackupRegion != "" {
		awsCfg.Region = cfg.S3BackupRegion
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3BackupEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3BackupEndpoint)
		}
		if cfg.S3BackupPathStyleAccess {
			o.UsePathStyle = true
		}
	})

	return &S3BackupFetcher{
		client:     client,
		bucketName: cfg.S3BackupBucket,
		prefix:     cfg.S3BackupPrefix,
	}, nil
}

// FetchBackup fetches the backup file from S3 for a given key fingerprint.
func (f *S3BackupFetcher) FetchBackup(ctx context.Context, keyFingerprint string) (io.ReadCloser, error) {
	key := path.Join(keyFingerprint, "backup.json")
	if f.prefix != "" {
		key = path.Join(f.prefix, key)
	}

	key = strings.TrimPrefix(key, "/") // Ensure no leading slash for S3 key if prefix was empty/root

	resp, err := f.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(f.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}
