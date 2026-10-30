package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/figchain/go-client/pkg/transport"
)

type failingStrategy struct{ err error }

func (s failingStrategy) Bootstrap(context.Context, []string) (*Result, error) { return nil, s.err }

func TestFallbackStrategy_UsesBackupWhenServerUnreachable(t *testing.T) {
	backup := &Result{Cursors: map[string]string{"ns": "backup"}}
	s := NewFallbackStrategy(failingStrategy{errors.New("connection refused")}, staticStrategy{backup})

	got, err := s.Bootstrap(context.Background(), []string{"ns"})
	if err != nil || got != backup {
		t.Fatalf("expected backup result, got %v, %v", got, err)
	}
}

func TestFallbackStrategy_FailsOnRejectedCredentials(t *testing.T) {
	for _, code := range []int{401, 403} {
		serverErr := fmt.Errorf("failed to fetch initial data for ns: %w", &transport.StatusError{StatusCode: code})
		s := NewFallbackStrategy(failingStrategy{serverErr}, staticStrategy{&Result{}})

		got, err := s.Bootstrap(context.Background(), []string{"ns"})
		var statusErr *transport.StatusError
		if got != nil || !errors.As(err, &statusErr) || statusErr.StatusCode != code {
			t.Fatalf("%d: expected the status error, got %v, %v", code, got, err)
		}
	}
}
