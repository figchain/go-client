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
		return false
	}

	// Generated bindings use string for Operator (enum)
	switch condition.Operator {
	case "EQUALS":
		if len(condition.Values) > 0 {
			return val == condition.Values[0]
		}
		return false
	case "NOT_EQUALS":
		if len(condition.Values) > 0 {
			return val != condition.Values[0]
		}
		return true
	case "IN":
		return slices.Contains(condition.Values, val)
	case "NOT_IN":
		return !slices.Contains(condition.Values, val)
	case "CONTAINS":
		if len(condition.Values) > 0 {
			return strings.Contains(val, condition.Values[0])
		}
		return false
	case "GREATER_THAN":
		if len(condition.Values) != 1 {
			return false
		}
		return e.compare(val, condition.Values[0]) > 0
	case "LESS_THAN":
		if len(condition.Values) != 1 {
			return false
		}
		return e.compare(val, condition.Values[0]) < 0
	case "SPLIT":
		if len(condition.Values) == 0 {
			return false
		}
		threshold, err := strconv.Atoi(condition.Values[0])
		if err != nil {
			return false
		}
		return e.getBucket(val) < threshold
	default:
		return false
	}
}

func (e *RuleBasedEvaluator) getBucket(key string) int {
	hash := uint32(0x811c9dc5)
	const prime = 0x01000193
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime
	}
	return int(hash % 100)
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
