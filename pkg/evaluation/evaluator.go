package evaluation

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/figchain/go-client/pkg/model"
)

// EvaluationContext holds the context for rule evaluation.
type EvaluationContext struct {
	Attributes map[string]string
}

// NewEvaluationContext creates a new EvaluationContext.
func NewEvaluationContext(attributes map[string]string) *EvaluationContext {
	if attributes == nil {
		attributes = make(map[string]string)
	}
	return &EvaluationContext{
		Attributes: attributes,
	}
}

// Merge merges another context into this one.
func (c *EvaluationContext) Merge(other *EvaluationContext) *EvaluationContext {
	merged := make(map[string]string)
	maps.Copy(merged, c.Attributes)
	if other != nil {
		maps.Copy(merged, other.Attributes)
	}
	return &EvaluationContext{Attributes: merged}
}

// Evaluator defines the interface for evaluating rollouts.
type Evaluator interface {
	Evaluate(figFamily *model.FigFamily, context *EvaluationContext) (*model.Fig, error)
}

// RuleBasedEvaluator implements rule-based rollout evaluation.
type RuleBasedEvaluator struct{}

// NewRuleBasedEvaluator creates a new RuleBasedEvaluator.
func NewRuleBasedEvaluator() *RuleBasedEvaluator {
	return &RuleBasedEvaluator{}
}

func (e *RuleBasedEvaluator) Evaluate(figFamily *model.FigFamily, context *EvaluationContext) (*model.Fig, error) {
	if figFamily == nil {
		return nil, fmt.Errorf("figFamily cannot be nil")
	}

	// 1. Check rules
	for _, rule := range figFamily.Rules {
		if e.matchesRule(rule, context) {
			return e.findFigByVersion(figFamily, rule.TargetVersion)
		}
	}

	// 2. Return default version
	if figFamily.DefaultVersion != nil {
		return e.findFigByVersion(figFamily, *figFamily.DefaultVersion)
	}

	return nil, nil
}

func (e *RuleBasedEvaluator) matchesRule(rule model.Rule, context *EvaluationContext) bool {
	for _, condition := range rule.Conditions {
		if !e.matchesCondition(condition, context) {
			return false
		}
	}
	return true
}

func (e *RuleBasedEvaluator) matchesCondition(condition model.Condition, context *EvaluationContext) bool {
	val, ok := context.Attributes[condition.Variable]
	if !ok {
		// If variable is missing, condition fails (unless operator handles missing, but usually fails)
		return false
	}

	switch condition.Operator {
	case model.OperatorEquals:
		return slices.Contains(condition.Values, val)
	case model.OperatorNotEquals:
		return !slices.Contains(condition.Values, val)
	case model.OperatorIn:
		return slices.Contains(condition.Values, val)
	case model.OperatorNotIn:
		return !slices.Contains(condition.Values, val)
	case model.OperatorContains:
		for _, v := range condition.Values {
			if strings.Contains(val, v) {
				return true
			}
		}
		return false
	case model.OperatorGreaterThan:
		if len(condition.Values) != 1 {
			return false
		}
		return e.compare(val, condition.Values[0]) > 0
	case model.OperatorLessThan:
		if len(condition.Values) != 1 {
			return false
		}
		return e.compare(val, condition.Values[0]) < 0
	case model.OperatorSplit:
		// TODO: Implement traffic splitting
		return false
	default:
		return false
	}
}

func (e *RuleBasedEvaluator) compare(a, b string) int {
	// Try numeric comparison first
	f1, err1 := strconv.ParseFloat(a, 64)
	f2, err2 := strconv.ParseFloat(b, 64)
	if err1 == nil && err2 == nil {
		if f1 < f2 {
			return -1
		}
		if f1 > f2 {
			return 1
		}
		return 0
	}
	// Fallback to string comparison
	return strings.Compare(a, b)
}

func (e *RuleBasedEvaluator) findFigByVersion(figFamily *model.FigFamily, version string) (*model.Fig, error) {
	for _, fig := range figFamily.Figs {
		if fig.Version == version {
			return &fig, nil
		}
	}
	return nil, fmt.Errorf("fig version %s not found", version)
}
