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

package addonphase

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/conditions"
	"addons-operator/internal/controller/rules"
)

const (
	phaseFinalizerName  = "addons.in-cloud.io/phase-cleanup"
	phaseRequeueAfter   = 30 * time.Second
	phaseDependencyName = "depIndex"
	phaseSetupTimeout   = 30 * time.Second
)

// AddonPhaseReconciler reconciles AddonPhase objects.
type AddonPhaseReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addonphases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addonphases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addonphases/finalizers,verbs=update
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addons,verbs=get;list;watch
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addons/status,verbs=get;update;patch

func (r *AddonPhaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	phase := &addonsv1alpha1.AddonPhase{}
	if err := r.Get(ctx, req.NamespacedName, phase); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	logger.V(1).Info("Reconciling AddonPhase", "rules", len(phase.Spec.Rules))

	cm := conditions.NewManager(&phase.Status.Conditions, phase.Generation)
	cm.EnsureAllConditions()

	if !phase.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, phase)
	}

	if err := r.ensureFinalizer(ctx, phase); err != nil {
		return ctrl.Result{}, err
	}

	addon := &addonsv1alpha1.Addon{}
	if err := r.Get(ctx, req.NamespacedName, addon); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Target Addon not found, waiting for creation", "addon", phase.Name)
			cm.SetProgressing(conditions.ReasonTargetAddonNotFound, conditions.ReasonTargetAddonNotFound,
				fmt.Sprintf("Addon %s not found", phase.Name))

			return r.updateStatusNoRequeue(ctx, phase)
		}

		return ctrl.Result{}, err
	}

	evaluator := rules.NewRuleEvaluator(r.Client)
	ruleStatuses, activeSelectors, err := evaluator.EvaluateRules(ctx, phase, addon)
	if err != nil {
		logger.Error(nil, "Failed to evaluate rules", "phase", phase.Name, "reason", err.Error())
		cm.SetProgressing(conditions.ReasonEvaluationFailed, conditions.ReasonEvaluationFailed, err.Error())

		return r.updateStatusAndRequeue(ctx, phase)
	}

	if err := r.patchAddonStatus(ctx, addon, activeSelectors); err != nil {
		logger.Error(nil, "Failed to patch Addon status", "addon", addon.Name, "reason", err.Error())
		cm.SetProgressing(conditions.ReasonPatchFailed, conditions.ReasonPatchFailed,
			"Failed to update Addon with phase selectors")

		return r.updateStatusAndRequeue(ctx, phase)
	}

	phase.Status.RuleStatuses = ruleStatuses

	activeCount := 0
	for _, rs := range ruleStatuses {
		if rs.Matched {
			activeCount++
		}
	}

	cm.SetReady(conditions.ReasonFullyReconciled,
		fmt.Sprintf("%d of %d rules active", activeCount, len(ruleStatuses)))

	r.Recorder.Eventf(phase, "Normal", "Reconciled",
		"Evaluated %d rules, %d active", len(ruleStatuses), activeCount)

	return r.updateStatus(ctx, phase, cm)
}

func (r *AddonPhaseReconciler) reconcileDelete(ctx context.Context, phase *addonsv1alpha1.AddonPhase) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling AddonPhase deletion")

	if controllerutil.ContainsFinalizer(phase, phaseFinalizerName) {
		if err := r.clearAddonStatus(ctx, phase.Name); err != nil {
			return ctrl.Result{}, err
		}

		phaseKey := client.ObjectKeyFromObject(phase)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			fresh := &addonsv1alpha1.AddonPhase{}
			if getErr := r.Get(ctx, phaseKey, fresh); getErr != nil {
				return getErr
			}
			controllerutil.RemoveFinalizer(fresh, phaseFinalizerName)

			return r.Update(ctx, fresh)
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		r.Recorder.Event(phase, "Normal", "Deleted", "AddonPhase deleted, cleaned up Addon status")
	}

	return ctrl.Result{}, nil
}

func (r *AddonPhaseReconciler) ensureFinalizer(ctx context.Context, phase *addonsv1alpha1.AddonPhase) error {
	if !controllerutil.ContainsFinalizer(phase, phaseFinalizerName) {
		phaseKey := client.ObjectKeyFromObject(phase)

		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			fresh := &addonsv1alpha1.AddonPhase{}
			if err := r.Get(ctx, phaseKey, fresh); err != nil {
				return err
			}
			if controllerutil.ContainsFinalizer(fresh, phaseFinalizerName) {
				return nil
			}
			controllerutil.AddFinalizer(fresh, phaseFinalizerName)

			return r.Update(ctx, fresh)
		})
	}

	return nil
}

func (r *AddonPhaseReconciler) patchAddonStatus(ctx context.Context, addon *addonsv1alpha1.Addon, selectors []addonsv1alpha1.ValuesSelector) error {
	addonKey := client.ObjectKeyFromObject(addon)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh := &addonsv1alpha1.Addon{}
		if err := r.Get(ctx, addonKey, fresh); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			return err
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		fresh.Status.PhaseValuesSelector = selectors

		return r.Status().Patch(ctx, fresh, patch)
	})
}

func (r *AddonPhaseReconciler) clearAddonStatus(ctx context.Context, name string) error {
	addonKey := client.ObjectKey{Name: name}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		addon := &addonsv1alpha1.Addon{}
		if err := r.Get(ctx, addonKey, addon); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			return err
		}
		patch := client.MergeFrom(addon.DeepCopy())
		addon.Status.PhaseValuesSelector = nil

		return r.Status().Patch(ctx, addon, patch)
	})
}

func hasNonAddonSources(phase *addonsv1alpha1.AddonPhase) bool {
	for _, rule := range phase.Spec.Rules {
		for _, criterion := range rule.Criteria {
			if criterion.Source != nil {
				if criterion.Source.APIVersion != addonsv1alpha1.GroupVersion.String() || criterion.Source.Kind != addonsv1alpha1.AddonKind {
					return true
				}
			}
		}
	}

	return false
}

func (r *AddonPhaseReconciler) updateStatus(ctx context.Context, phase *addonsv1alpha1.AddonPhase, cm *conditions.Manager) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	phase.Status.ObservedGeneration = phase.Generation

	if err := r.Status().Update(ctx, phase); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict updating AddonPhase status, will retry", "addonphase", phase.Name)

			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to update AddonPhase status")

		return ctrl.Result{}, err
	}

	// Requeue only if has non-Addon sources (no watches for them)
	if cm.IsReady() && hasNonAddonSources(phase) {
		return ctrl.Result{RequeueAfter: phaseRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func (r *AddonPhaseReconciler) updateStatusNoRequeue(ctx context.Context, phase *addonsv1alpha1.AddonPhase) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	phase.Status.ObservedGeneration = phase.Generation

	if err := r.Status().Update(ctx, phase); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict updating AddonPhase status, will retry", "addonphase", phase.Name)

			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to update AddonPhase status")

		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AddonPhaseReconciler) updateStatusAndRequeue(ctx context.Context, phase *addonsv1alpha1.AddonPhase) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	phase.Status.ObservedGeneration = phase.Generation

	if err := r.Status().Update(ctx, phase); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict updating AddonPhase status, will retry", "addonphase", phase.Name)

			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to update AddonPhase status")

		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: phaseRequeueAfter}, nil
}

func (r *AddonPhaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx, cancel := context.WithTimeout(context.Background(), phaseSetupTimeout)
	defer cancel()

	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&addonsv1alpha1.AddonPhase{},
		phaseDependencyName,
		func(obj client.Object) []string {
			phase, ok := obj.(*addonsv1alpha1.AddonPhase)
			if !ok {
				return nil
			}
			deps := make(map[string]struct{})
			for _, rule := range phase.Spec.Rules {
				for _, criterion := range rule.Criteria {
					if criterion.Source != nil && criterion.Source.Name != "" {
						deps[criterion.Source.Name] = struct{}{}
					}
				}
			}
			result := make([]string, 0, len(deps))
			for dep := range deps {
				result = append(result, dep)
			}

			return result
		},
	); err != nil {
		return fmt.Errorf("create dependency index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.AddonPhase{}).
		Watches(
			&addonsv1alpha1.Addon{},
			handler.EnqueueRequestsFromMapFunc(r.findPhaseByAddonName),
		).
		Watches(
			&addonsv1alpha1.Addon{},
			handler.EnqueueRequestsFromMapFunc(r.findPhasesByDependency),
		).
		Named("addonphase").
		Complete(r)
}

func (r *AddonPhaseReconciler) findPhaseByAddonName(ctx context.Context, obj client.Object) []reconcile.Request {
	addon, ok := obj.(*addonsv1alpha1.Addon)
	if !ok {
		return nil
	}

	phase := &addonsv1alpha1.AddonPhase{}
	if err := r.Get(ctx, types.NamespacedName{Name: addon.Name}, phase); err != nil {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Name: addon.Name},
	}}
}

func (r *AddonPhaseReconciler) findPhasesByDependency(ctx context.Context, obj client.Object) []reconcile.Request {
	addon, ok := obj.(*addonsv1alpha1.Addon)
	if !ok {
		return nil
	}

	key := addon.Name

	var phases addonsv1alpha1.AddonPhaseList
	if err := r.List(ctx, &phases, client.MatchingFields{
		phaseDependencyName: key,
	}); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(phases.Items))
	for _, phase := range phases.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: phase.Name},
		})
	}

	return requests
}
