package rules

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Rule represents a single CEL evaluation rule
type Rule struct {
	Operation  string `yaml:"operation" json:"operation"`   // Name of the operation (e.g., "Migrating")
	Expression string `yaml:"expression" json:"expression"` // CEL expression to evaluate
}

// GroupVersionKind identifies a Kubernetes resource type
type GroupVersionKind struct {
	Group   string `yaml:"group" json:"group"`
	Version string `yaml:"version" json:"version"`
	Kind    string `yaml:"kind" json:"kind"`
}

// ToSchemaGVK converts to k8s schema.GroupVersionKind
func (g GroupVersionKind) ToSchemaGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   g.Group,
		Version: g.Version,
		Kind:    g.Kind,
	}
}

// RuleSet represents the complete rules file structure
type RuleSet struct {
	Name       string
	GVK        GroupVersionKind `yaml:"gvk" json:"gvk"`
	Rules      []Rule           `yaml:"rules" json:"rules"`
	Namespaces []string         `yaml:"namespaces" json:"namespaces"`
}

// Validate validates the rules file structure
func (r *RuleSet) Validate() error {
	// Validate GVK
	if r.GVK.Kind == "" {
		return fmt.Errorf("GVK.kind is required")
	}
	if r.GVK.Version == "" {
		return fmt.Errorf("GVK.version is required")
	}
	// Group can be empty for core resources

	// Validate rules
	if len(r.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	for i, rule := range r.Rules {
		if rule.Operation == "" {
			return fmt.Errorf("rule[%d].operation is required", i)
		}
		if rule.Expression == "" {
			return fmt.Errorf("rule[%d].expression is required", i)
		}
	}

	return nil
}
