package evaluator

import (
	"fmt"

	api "github.com/ifo-operator/inflightoperations/api/v1alpha1"
	"github.com/ifo-operator/inflightoperations/internal/rules"
	liberr "github.com/ifo-operator/inflightoperations/lib/error"
)

type RuleSetResult struct {
	RuleSet    *api.OperationRuleSet
	Operations []string
}

// Evaluator provides CEL expression evaluation for Kubernetes resources
type Evaluator interface {
	// Evaluate evaluates a single CEL expression against an unstructured object
	Evaluate(subject *api.Subject, rule *api.Rule) (bool, error)

	// EvaluateRuleSet evaluates a RuleSet and returns as RuleSetResult.
	EvaluateRuleSet(subject *api.Subject, ruleset *api.OperationRuleSet) (RuleSetResult, error)
}

// celEvaluator implements the Evaluator interface
type celEvaluator struct {
	programCache *ProgramCache
}

// NewEvaluator creates a new CEL evaluator with program caching
func NewEvaluator() (Evaluator, error) {
	cache, err := NewProgramCache()
	if err != nil {
		return nil, fmt.Errorf("failed to create program cache: %w", err)
	}

	return &celEvaluator{
		programCache: cache,
	}, nil
}

func (r *celEvaluator) ShouldEvaluate(subject *api.Subject, ruleset rules.RuleSet) bool {
	if len(ruleset.Namespaces) == 0 {
		return true
	}
	for _, namespace := range ruleset.Namespaces {
		if subject.GetNamespace() == namespace {
			return true
		}
	}
	return false
}

// Evaluate evaluates a single CEL expression against an unstructured object
func (r *celEvaluator) Evaluate(subject *api.Subject, rule *api.Rule) (value bool, err error) {
	prog, err := r.programCache.GetOrCompile(rule.Expression)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}

	vars := map[string]interface{}{
		"object": subject.Object,
	}

	out, _, err := prog.Eval(vars)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}

	// Convert result to boolean
	value, ok := out.Value().(bool)
	if !ok {
		err = fmt.Errorf("expression did not return a boolean value, got: %v (type: %T)", out.Value(), out.Value())
		return
	}
	return
}

// EvaluateRules evaluates all rules and returns matching operation names
func (r *celEvaluator) EvaluateRuleSet(subject *api.Subject, ruleset *api.OperationRuleSet) (result RuleSetResult, err error) {
	result = RuleSetResult{
		RuleSet: ruleset,
	}
	for _, rule := range ruleset.Rules() {
		var matched bool
		matched, err = r.Evaluate(subject, &rule)
		if err != nil {
			err = liberr.Wrap(err)
			return
		}
		if matched {
			result.Operations = append(result.Operations, rule.Operation)
		}
	}
	return
}
