/*
Generated-by: Claude Code
*/
package watch

import (
	"fmt"
	"sync"
	"testing"

	"github.com/ifo-operator/inflightoperations/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRuleCacheAddAndGet(t *testing.T) {
	cache := NewRuleCache()

	rule := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rule"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Migrating", Expression: "true"},
			},
		},
	}

	cache.AddOrUpdateRule(&rule)

	gvk := schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	rules := cache.List(gvk)
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}
	if rules[0].Name != "test-rule" {
		t.Errorf("Expected rule name 'test-rule', got '%s'", rules[0].Name)
	}
}

func TestRuleCacheUpdate(t *testing.T) {
	cache := NewRuleCache()

	// Add initial rule
	rule := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rule"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Migrating", Expression: "true"},
			},
		},
	}
	cache.AddOrUpdateRule(&rule)

	// Update with new version
	updatedRule := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rule"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Starting", Expression: "true"},
			},
		},
	}
	cache.AddOrUpdateRule(&updatedRule)

	gvk := schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	rules := cache.List(gvk)
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule after update, got %d", len(rules))
	}
	if len(rules[0].Spec.Rules) != 1 || rules[0].Spec.Rules[0].Operation != "Starting" {
		t.Errorf("Rule was not updated correctly")
	}
}

func TestRuleCacheRemove(t *testing.T) {
	cache := NewRuleCache()

	rule := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rule"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Migrating", Expression: "true"},
			},
		},
	}
	cache.AddOrUpdateRule(&rule)

	gvk := schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	cache.RemoveRule(&rule)

	rules := cache.List(gvk)
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules after removal, got %d", len(rules))
	}
}

func TestRuleCacheMultipleRulesPerGVK(t *testing.T) {
	cache := NewRuleCache()

	rule1 := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rule-1"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Migrating", Expression: "true"},
			},
		},
	}

	rule2 := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rule-2"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Starting", Expression: "true"},
			},
		},
	}

	cache.AddOrUpdateRule(&rule1)
	cache.AddOrUpdateRule(&rule2)

	gvk := schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	rules := cache.List(gvk)
	if len(rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rules))
	}
}

func TestRuleCacheConcurrency(t *testing.T) {
	cache := NewRuleCache()
	var wg sync.WaitGroup

	// Simulate concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rule := v1alpha1.OperationRuleSet{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("rule-%d", id)},
				Spec: v1alpha1.OperationRuleSetSpec{
					Target: v1alpha1.GroupVersionKind{
						Group:   "test.io",
						Version: "v1",
						Kind:    "TestResource",
					},
					Rules: []v1alpha1.Rule{
						{Operation: "TestOp", Expression: "true"},
					},
				},
			}
			cache.AddOrUpdateRule(&rule)
		}(i)
	}

	// Simulate concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gvk := schema.GroupVersionKind{
				Group:   "test.io",
				Version: "v1",
				Kind:    "TestResource",
			}
			_ = cache.List(gvk)
		}()
	}

	wg.Wait()

	// Verify final state
	gvk := schema.GroupVersionKind{
		Group:   "test.io",
		Version: "v1",
		Kind:    "TestResource",
	}
	rules := cache.List(gvk)
	if len(rules) != 10 {
		t.Errorf("Expected 10 rules after concurrent operations, got %d", len(rules))
	}
}

func TestRuleCacheGetAllGVKs(t *testing.T) {
	cache := NewRuleCache()

	rule1 := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rule-1"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Migrating", Expression: "true"},
			},
		},
	}

	rule2 := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rule-2"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "cdi.kubevirt.io",
				Version: "v1beta1",
				Kind:    "DataVolume",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Provisioning", Expression: "true"},
			},
		},
	}

	cache.AddOrUpdateRule(&rule1)
	cache.AddOrUpdateRule(&rule2)

	gvks := cache.GVKs()
	if len(gvks) != 2 {
		t.Errorf("Expected 2 GVKs, got %d", len(gvks))
	}
}

func TestRuleCacheImmutability(t *testing.T) {
	cache := NewRuleCache()

	rule := v1alpha1.OperationRuleSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rule"},
		Spec: v1alpha1.OperationRuleSetSpec{
			Target: v1alpha1.GroupVersionKind{
				Group:   "kubevirt.io",
				Version: "v1",
				Kind:    "VirtualMachine",
			},
			Rules: []v1alpha1.Rule{
				{Operation: "Migrating", Expression: "true"},
			},
		},
	}
	cache.AddOrUpdateRule(&rule)

	gvk := schema.GroupVersionKind{
		Group:   "kubevirt.io",
		Version: "v1",
		Kind:    "VirtualMachine",
	}

	// Get rules
	rules := cache.List(gvk)

	// Modify the returned slice (should not affect cache)
	rules[0].Name = "test-immutability"

	// Verify cache is unchanged
	rulesAfter := cache.List(gvk)
	if len(rulesAfter) != 1 || rulesAfter[0].Name != "test-rule" {
		t.Errorf("Cache was modified by external slice modification")
	}
}
