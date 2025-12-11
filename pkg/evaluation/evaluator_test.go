package evaluation

import (
	"testing"

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
