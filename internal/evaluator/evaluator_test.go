package evaluator

import (
	"testing"

	api "github.com/ifo-operator/inflightoperations/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeObject(data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: data,
	}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name    string
		object  *unstructured.Unstructured
		rule    *api.Rule
		want    bool
		wantErr bool
	}{
		{
			name: "simple string equality - match",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			}),
			rule:    &api.Rule{Expression: "object.status.phase == 'Running'"},
			want:    true,
			wantErr: false,
		},
		{
			name: "simple string equality - no match",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Pending",
				},
			}),
			rule:    &api.Rule{Expression: "object.status.phase == 'Running'"},
			want:    false,
			wantErr: false,
		},
		{
			name: "nested field access",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"printableStatus": "Migrating",
				},
			}),
			rule:    &api.Rule{Expression: "object.status.printableStatus == 'Migrating'"},
			want:    true,
			wantErr: false,
		},
		{
			name: "numeric comparison",
			object: makeObject(map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": 3,
				},
			}),
			rule:    &api.Rule{Expression: "object.spec.replicas > 2"},
			want:    true,
			wantErr: false,
		},
		{
			name: "logical AND",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
					"ready": true,
				},
			}),
			rule:    &api.Rule{Expression: "object.status.phase == 'Running' && object.status.ready == true"},
			want:    true,
			wantErr: false,
		},
		{
			name: "logical OR",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Pending",
				},
			}),
			rule:    &api.Rule{Expression: "object.status.phase == 'Running' || object.status.phase == 'Pending'"},
			want:    true,
			wantErr: false,
		},
		{
			name: "invalid expression syntax",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			}),
			rule:    &api.Rule{Expression: "object.status.phase =="},
			want:    false,
			wantErr: true,
		},
		{
			name: "non-boolean expression",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			}),
			rule:    &api.Rule{Expression: "object.status.phase"},
			want:    false,
			wantErr: true,
		},
	}

	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() failed: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := eval.Evaluate(tt.object, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateRules(t *testing.T) {
	tests := []struct {
		name    string
		object  *unstructured.Unstructured
		rules   []api.Rule
		want    []string
		wantErr bool
	}{
		{
			name: "single matching rule",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"printableStatus": "Migrating",
				},
			}),
			rules: []api.Rule{
				{Operation: "Migrating", Expression: "object.status.printableStatus == 'Migrating'"},
				{Operation: "Starting", Expression: "object.status.printableStatus == 'Starting'"},
			},
			want:    []string{"Migrating"},
			wantErr: false,
		},
		{
			name: "multiple matching rules",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
					"ready": true,
				},
			}),
			rules: []api.Rule{
				{Operation: "Running", Expression: "object.status.phase == 'Running'"},
				{Operation: "Ready", Expression: "object.status.ready == true"},
				{Operation: "Pending", Expression: "object.status.phase == 'Pending'"},
			},
			want:    []string{"Running", "Ready"},
			wantErr: false,
		},
		{
			name: "no matching rules",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"printableStatus": "Running",
				},
			}),
			rules: []api.Rule{
				{Operation: "Migrating", Expression: "object.status.printableStatus == 'Migrating'"},
				{Operation: "Starting", Expression: "object.status.printableStatus == 'Starting'"},
			},
			want:    []string{},
			wantErr: false,
		},
		{
			name: "empty rules list",
			object: makeObject(map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			}),
			rules:   []api.Rule{},
			want:    []string{},
			wantErr: false,
		},
	}

	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() failed: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ruleset := api.OperationRuleSet{}
			ruleset.Name = tt.name
			ruleset.Spec.Rules = tt.rules
			got, err := eval.EvaluateRuleSet(tt.object, &ruleset)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateRules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got.Operations) != len(tt.want) {
					t.Errorf("EvaluateRules() returned %d operations, want %d. Got: %v, Want: %v", len(got.Operations), len(tt.want), got, tt.want)
					return
				}

				for i, op := range got.Operations {
					if op != tt.want[i] {
						t.Errorf("EvaluateRules()[%d] = %q, want %q", i, op, tt.want[i])
					}
				}
			}
		})
	}
}

func TestProgramCaching(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() failed: %v", err)
	}

	obj := makeObject(map[string]interface{}{
		"status": map[string]interface{}{
			"phase": "Running",
		},
	})

	expression := "object.status.phase == 'Running'"
	rule := api.Rule{Expression: expression}

	// Evaluate the same expression multiple times
	for i := 0; i < 5; i++ {
		result, err := eval.Evaluate(obj, &rule)
		if err != nil {
			t.Fatalf("Evaluate() iteration %d failed: %v", i, err)
		}
		if !result {
			t.Errorf("Evaluate() iteration %d = false, want true", i)
		}
	}

	// The program should have been cached and reused
	// This test mainly verifies that caching doesn't break functionality
}
