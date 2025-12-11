package vault

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	fc_config "github.com/figchain/go-client/pkg/config"
)

// VaultFetcher defines the interface for fetching backup files.
type VaultFetcher interface {
	FetchBackup(ctx context.Context, keyFingerprint string) (io.ReadCloser, error)
}

// S3VaultFetcher fetches backup files from S3.
type S3VaultFetcher struct {
	client     *s3.Client
	bucketName string
	prefix     string
}

// NewS3VaultFetcher creates a new S3VaultFetcher.
func NewS3VaultFetcher(ctx context.Context, cfg *fc_config.Config) (*S3VaultFetcher, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	if cfg.VaultRegion != "" {
		awsCfg.Region = cfg.VaultRegion
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.VaultEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.VaultEndpoint)
		}
		if cfg.VaultPathStyle {
			o.UsePathStyle = true
		}
	})

	return &S3VaultFetcher{
		client:     client,
		bucketName: cfg.VaultBucket,
		prefix:     cfg.VaultPrefix,
	}, nil
}

// FetchBackup fetches the backup file from S3 for a given key fingerprint.
func (f *S3VaultFetcher) FetchBackup(ctx context.Context, keyFingerprint string) (io.ReadCloser, error) {
	key := "backup.json"
	if keyFingerprint != "" {
		key = keyFingerprint + "/" + key
	}

	if f.prefix != "" {
		if strings.HasSuffix(f.prefix, "/") {
			key = f.prefix + key
		} else {
			key = f.prefix + "/" + key
		}
	}

	key = strings.TrimPrefix(key, "/")

	resp, err := f.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(f.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch backup from S3: %w", err)
	}

	return resp.Body, nil
}
