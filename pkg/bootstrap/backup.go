package bootstrap

import (
	"context"

	"github.com/figchain/go-client/pkg/model"
	"github.com/figchain/go-client/pkg/backup"
)

// S3BackupStrategy implements bootstrapping from the FigChain S3Backup (S3).
type S3BackupStrategy struct {
	backupService *backup.S3BackupService
}

// NewS3BackupStrategy creates a new S3BackupStrategy.
func NewS3BackupStrategy(vs *backup.S3BackupService) *S3BackupStrategy {
	return &S3BackupStrategy{backupService: vs}
}

// Bootstrap loads data from the S3Backup.
func (s *S3BackupStrategy) Bootstrap(ctx context.Context, namespaces []string) (*Result, error) {
	payload, err := s.backupService.LoadBackup(ctx)
	if err != nil {
		return nil, err
	}

	cursors := make(map[string]string)
	if payload.SyncToken != "" {
		for _, ns := range namespaces {
			cursors[ns] = payload.SyncToken
		}
	}

	for _, item := range payload.Items {
		ns := item.Definition.Namespace
		if _, ok := cursors[ns]; !ok {
			cursors[ns] = payload.SyncToken
		}
	}

	// Filter Items to relevant namespaces
	requestedNamespaces := make(map[string]struct{}, len(namespaces))
	for _, ns := range namespaces {
		requestedNamespaces[ns] = struct{}{}
	}

	filteredFamilies := make([]model.FigFamily, 0)
	for _, item := range payload.Items {
		if _, ok := requestedNamespaces[item.Definition.Namespace]; ok {
			filteredFamilies = append(filteredFamilies, item)
		}
	}

	return &Result{
		FigFamilies: filteredFamilies,
		Cursors:     cursors,
	}, nil
}

func (s *S3BackupStrategy) String() string {
	return "S3BackupStrategy"
}

var _ Strategy = (*S3BackupStrategy)(nil)
