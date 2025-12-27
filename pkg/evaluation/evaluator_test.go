package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/figchain/go-client/pkg/model"
)

func TestRuleBasedEvaluator_Evaluate(t *testing.T) {
	evaluator := NewRuleBasedEvaluator()

	defaultVersion := "v1"
	targetVersion := "v2"
	description := "test rule"

	figFamily := &model.FigFamily{
		DefaultVersion: &defaultVersion,
		Figs: []model.Fig{
			{Version: "v1", Payload: []byte("v1")},
			{Version: "v2", Payload: []byte("v2")},
		},
		Rules: []model.Rule{
			{
				Description:   &description,
				TargetVersion: targetVersion,
				Conditions: []model.Condition{
					{
						Variable: "user_id",
						Operator: "EQUALS",
						Values:   []string{"123"},
					},
				},
			},
		},
	}

	tests := []struct {
		name    string
		context *EvaluationContext
		want    string
	}{
		{
			name:    "match rule",
			context: NewEvaluationContext(map[string]string{"user_id": "123"}),
			want:    "v2",
		},
		{
			name:    "no match rule",
			context: NewEvaluationContext(map[string]string{"user_id": "456"}),
			want:    "v1",
		},
		{
			name:    "missing variable",
			context: NewEvaluationContext(map[string]string{"other": "123"}),
			want:    "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluator.Evaluate(figFamily, tt.context)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
				return
			}
			if got.Version != tt.want {
				t.Errorf("Evaluate() got = %v, want %v", got.Version, tt.want)
			}
		})
	}
}

func TestEvaluationContext_ImplementsContext(t *testing.T) {
	// Test that EvaluationContext properly implements context.Context
	attributes := map[string]string{"user": "test"}
	evalCtx := NewEvaluationContext(attributes)

	// Should implement context.Context
	var _ context.Context = evalCtx

	// Test that it has the expected attributes
	if evalCtx.Attributes["user"] != "test" {
		t.Errorf("Expected attribute 'user' to be 'test', got '%s'", evalCtx.Attributes["user"])
	}

	// Test that context methods work
	if evalCtx.Err() != nil {
		t.Errorf("Fresh context should not have an error")
	}
	if _, ok := evalCtx.Deadline(); ok {
		t.Errorf("Background-based context should not have a deadline")
	}
}

func TestEvaluationContext_WithTimeout(t *testing.T) {
	// Test that we can create an EvaluationContext with a timeout
	baseCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	evalCtx := NewEvaluationContextWithContext(baseCtx, map[string]string{
		"user": "test",
	})

	// Should have a deadline
	deadline, ok := evalCtx.Deadline()
	if !ok {
		t.Errorf("Context with timeout should have a deadline")
	}
	if deadline.IsZero() {
		t.Errorf("Deadline should not be zero")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should now be cancelled
	if evalCtx.Err() == nil {
		t.Errorf("Context should be cancelled after timeout")
	}
	if evalCtx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded error, got %v", evalCtx.Err())
	}

	// Attributes should still be accessible
	if evalCtx.Attributes["user"] != "test" {
		t.Errorf("Attributes should still be accessible after timeout")
	}
}

func TestEvaluationContext_WithCancel(t *testing.T) {
	// Test that cancellation propagates correctly
	baseCtx, cancel := context.WithCancel(context.Background())

	evalCtx := NewEvaluationContextWithContext(baseCtx, map[string]string{
		"user": "test",
	})

	// Should not be cancelled yet
	if evalCtx.Err() != nil {
		t.Errorf("Context should not be cancelled yet")
	}

	// Cancel the base context
	cancel()

	// Should now be cancelled
	select {
	case <-evalCtx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Context should be cancelled immediately")
	}

	if evalCtx.Err() != context.Canceled {
		t.Errorf("Expected Canceled error, got %v", evalCtx.Err())
	}
}

func TestEvaluationContext_Merge(t *testing.T) {
	ctx1 := NewEvaluationContext(map[string]string{
		"user":   "alice",
		"region": "us-west",
	})

	ctx2 := NewEvaluationContext(map[string]string{
		"region": "eu-west",
		"tier":   "premium",
	})

	merged := ctx1.Merge(ctx2)

	// Should have user from ctx1
	if merged.Attributes["user"] != "alice" {
		t.Errorf("Expected user 'alice', got '%s'", merged.Attributes["user"])
	}

	// Should have region from ctx2 (overwrite)
	if merged.Attributes["region"] != "eu-west" {
		t.Errorf("Expected region 'eu-west', got '%s'", merged.Attributes["region"])
	}

	// Should have tier from ctx2
	if merged.Attributes["tier"] != "premium" {
		t.Errorf("Expected tier 'premium', got '%s'", merged.Attributes["tier"])
	}

	// Should preserve the original context by having same deadline behavior
	_, ok1 := ctx1.Deadline()
	_, ok2 := merged.Deadline()
	if ok1 != ok2 {
		t.Errorf("Merged context should preserve the original context behavior")
	}
}

