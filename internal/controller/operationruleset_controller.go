/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/ifo-operator/inflightoperations/internal/evaluator"
	"github.com/ifo-operator/inflightoperations/internal/watch"
	libcnd "github.com/ifo-operator/inflightoperations/lib/condition"
	liberr "github.com/ifo-operator/inflightoperations/lib/error"
	"github.com/ifo-operator/inflightoperations/lib/logging"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/ifo-operator/inflightoperations/api/v1alpha1"
)

const (
	Name = "operationrule"
)

// OperationRuleReconciler reconciles a OperationRuleSet object
type OperationRuleReconciler struct {
	BaseReconciler
	client.Client
	Scheme          *runtime.Scheme
	DiscoveryClient *discovery.DiscoveryClient
	DynamicClient   dynamic.Interface
	Rules           *watch.RuleCache
	Watcher         *watch.Watcher
	Evaluator       evaluator.Evaluator
}

// +kubebuilder:rbac:groups=ifo-operator.org,resources=operationrulesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ifo-operator.org,resources=operationrulesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ifo-operator.org,resources=operationrulesets/finalizers,verbs=update
// +kubebuilder:rbac:groups=ifo-operator.org,resources=inflightoperations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ifo-operator.org,resources=inflightoperations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ifo-operator.org,resources=inflightoperations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OperationRuleSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its RuleSetResult here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *OperationRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	r.Log = logging.WithName(names.SimpleNameGenerator.GenerateName(Name+"|"), "operationrule", req)
	r.Started()
	defer func() {
		result.RequeueAfter = r.Ended(result.RequeueAfter, err)
		err = nil
	}()

	operationRule := &api.OperationRuleSet{}
	if err := r.Get(ctx, req.NamespacedName, operationRule); err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("OperationRuleSet not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		r.Log.Error(err, "Failed to get OperationRuleSet")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !operationRule.DeletionTimestamp.IsZero() {
		err := r.Teardown(ctx, operationRule)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = r.RemoveFinalizer(ctx, operationRule)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Info("Successfully finalized OperationRuleSet", "rule", operationRule.Name)
	}

	if operationRule.DeletionTimestamp.IsZero() {
		err := r.AddFinalizer(ctx, operationRule)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	operationRule.Status.BeginStagingConditions()
	r.Log.Info("Begin validating OperationRuleSet", "rule", operationRule.Name)
	err = r.Validate(ctx, operationRule)
	if err != nil {
		return
	}
	r.Log.Info("Done validating OperationRuleSet", "rule", operationRule.Name)

	// Ready condition.
	if !operationRule.Status.HasBlockerCondition() {
		err = r.Setup(ctx, operationRule)
		if err != nil {
			r.Log.Error(err, "Failed to ensure watch", "gvk", operationRule.GVK().String())
			operationRule.Status.SetCondition(libcnd.Condition{
				Type:    api.TypeWatchFailed,
				Status:  api.True,
				Reason:  api.ReasonWatchSetupFailed,
				Message: fmt.Sprintf("Failed to setup watch: %v", err),
			})
		} else {
			operationRule.Status.SetCondition(
				libcnd.Condition{
					Type:     api.TypeReady,
					Status:   api.True,
					Category: api.CategoryRequired,
					Reason:   api.ReasonWatchActive,
					Message:  fmt.Sprintf("Dynamic watch is active for %s", operationRule.GVK().String()),
				})
		}
	}
	return ctrl.Result{}, nil
}

func (r *OperationRuleReconciler) Validate(ctx context.Context, rule *api.OperationRuleSet) error {
	err := r.validateTargetGVK(rule)
	if err != nil {
		r.Log.Error(err, "Invalid target GVK", "gvk", rule.GVK().String())
		rule.Status.SetCondition(libcnd.Condition{
			Type:     api.TypeInvalidTarget,
			Status:   api.True,
			Reason:   api.ReasonGVKNotFound,
			Category: api.CategoryCritical,
			Message:  fmt.Sprintf("Target GVK does not exist: %v", err),
		})
	}

	err = r.validateCELExpressions(rule)
	if err != nil {
		r.Log.Error(err, "Invalid CEL expressions")
		rule.Status.SetCondition(libcnd.Condition{
			Type:     api.TypeInvalidRule,
			Status:   api.True,
			Reason:   api.ReasonInvalidExpression,
			Category: api.CategoryCritical,
			Message:  fmt.Sprintf("Invalid CEL expression: %v", err),
		})
	}

	if !rule.Status.HasBlockerCondition() {
		rule.Status.SetCondition(
			libcnd.Condition{
				Type:     api.TypeValidated,
				Status:   api.True,
				Reason:   api.ReasonCompleted,
				Category: api.CategoryAdvisory,
				Message:  "Validation has been completed.",
			})
	}

	return nil
}

func (r *OperationRuleReconciler) Setup(_ context.Context, rule *api.OperationRuleSet) error {
	r.Rules.AddOrUpdateRule(rule)
	err := r.Watcher.Register(rule.GVK())
	if err != nil {
		return err
	}
	return nil
}

func (r *OperationRuleReconciler) Teardown(_ context.Context, rule *api.OperationRuleSet) error {
	r.Rules.RemoveRule(rule)
	r.Watcher.Prune()
	return nil
}

// Initialize sets up the controller with the Manager.
func (r *OperationRuleReconciler) Initialize(mgr ctrl.Manager) error {
	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		err = liberr.Wrap(err)
		return err
	}
	eval, err := evaluator.NewEvaluator()
	if err != nil {
		err = liberr.Wrap(err)
		return err
	}
	rules := watch.NewRuleCache()
	watcher := watch.NewWatcher(
		r.Client,
		r.DynamicClient,
		r.DiscoveryClient,
		rules,
		eval,
		logging.WithName("watcher"),
	)
	r.DiscoveryClient = dc
	r.Evaluator = eval
	r.Rules = rules
	r.Watcher = watcher
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.OperationRuleSet{}).
		Named("operationrule").
		Complete(r)
}

func (r *OperationRuleReconciler) AddFinalizer(ctx context.Context, rule *api.OperationRuleSet) (err error) {
	patch := client.MergeFrom(rule.DeepCopy())
	if controllerutil.AddFinalizer(rule, api.OperationRuleSetFinalizer) {
		err = r.Patch(ctx, rule, patch)
		if err != nil {
			r.Log.Error(err, "failed to add finalizer", "rule", rule.Name, "namespace", rule.Namespace)
			return
		}
	}
	return
}

func (r *OperationRuleReconciler) RemoveFinalizer(ctx context.Context, rule *api.OperationRuleSet) (err error) {
	patch := client.MergeFrom(rule.DeepCopy())
	if controllerutil.RemoveFinalizer(rule, api.OperationRuleSetFinalizer) {
		err = r.Patch(ctx, rule, patch)
		if err != nil {
			r.Log.Error(err, "failed to remove finalizer", "rule", rule.Name, "namespace", rule.Namespace)
			return
		}
	}
	return
}

// validateTargetGVK checks if the specified GVK exists using discovery
func (r *OperationRuleReconciler) validateTargetGVK(or *api.OperationRuleSet) error {
	// Get all API resources for the group/version
	gvk := or.GVK()
	resourceList, err := r.DiscoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return fmt.Errorf("failed to discover resources for %s: %w", gvk.GroupVersion().String(), err)
	}

	// Check if the kind exists
	for _, resource := range resourceList.APIResources {
		if resource.Kind == gvk.Kind {
			return nil
		}
	}

	return fmt.Errorf("kind %s not found in %s", gvk.Kind, gvk.GroupVersion().String())
}

// validateCELExpressions validates all CEL expressions by attempting to compile them
func (r *OperationRuleReconciler) validateCELExpressions(or *api.OperationRuleSet) (err error) {
	for _, rule := range or.Rules() {
		// Try to compile the expression using the evaluator
		// We use a dummy object to test compilation
		dummyObj := map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "test",
			},
		}

		// The evaluator's internal compile will catch syntax errors
		_, err = r.Evaluator.Evaluate(
			&unstructured.Unstructured{Object: dummyObj},
			&rule,
		)
		if err != nil {
			return
		}
	}
	return
}
