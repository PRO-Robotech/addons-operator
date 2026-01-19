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
	"time"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	"addons-operator/internal/controller/argocd"
	"addons-operator/internal/controller/conditions"
	"addons-operator/internal/controller/dependencies"
	"addons-operator/internal/controller/dynamicwatch"
	"addons-operator/internal/controller/sources"
	"addons-operator/internal/controller/status"
	"addons-operator/internal/controller/values"
)

const (
	requeueIntervalDegraded = 60 * time.Second
	finalizerName           = "addons.in-cloud.io/finalizer"
	addonLabelKey           = "addons.in-cloud.io/addon"
	valuesSourcesIndexKey   = "spec.valuesSources.sourceRef"
	dependencyIndexKey      = "spec.initDependencies.name"
	setupTimeout            = 30 * time.Second
)

// AddonReconciler reconciles Addon objects.
type AddonReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Dynamic watch components for SourceRef watches
	watchManager dynamicwatch.WatchManager
	tracker      dynamicwatch.Tracker
	resolver     dynamicwatch.Resolver
}

// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addons/finalizers,verbs=update
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addonvalues,verbs=get;list;watch
// +kubebuilder:rbac:groups=argoproj.io,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="*",resources="*",verbs=get;list;watch

func (r *AddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	addon := &addonsv1alpha1.Addon{}
	if err := r.Get(ctx, req.NamespacedName, addon); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	cm := conditions.NewManager(&addon.Status.Conditions, addon.Generation)
	cm.EnsureAllConditions()

	logger.V(1).Info("Reconciling Addon", "chart", addon.Spec.Chart, "version", addon.Spec.Version)

	// Update dynamic watches based on current valuesSources
	if err := r.updateDynamicWatches(ctx, addon); err != nil {
		logger.Error(err, "failed to update dynamic watches")
		// Continue reconciliation - watches are best-effort
	}

	if !addon.DeletionTimestamp.IsZero() {
		cm.SetProgressing(conditions.ReasonDeleting, conditions.ReasonDeleting, "Addon is being deleted")
		return r.reconcileDelete(ctx, addon)
	}

	if err := r.ensureFinalizer(ctx, addon); err != nil {
		return ctrl.Result{}, err
	}

	cm.SetProgressing(conditions.ReasonInitializing, conditions.ReasonReconciling, "Reconciliation in progress")

	dependenciesMet, err := r.checkDependencies(ctx, addon)
	if err != nil {
		logger.Error(nil, "Failed to check dependencies", "addon", addon.Name, "reason", err.Error())
		cm.SetOperationalCondition(conditions.TypeDependenciesMet, false, "DependencyCheckFailed", err.Error())
		cm.SetProgressing(conditions.ReasonDependenciesNotMet, conditions.ReasonWaitingForDependency, "Failed to check dependencies")
		return r.updateStatus(ctx, addon, cm)
	}
	if !dependenciesMet {
		logger.V(1).Info("Waiting for dependencies")
		cm.SetOperationalCondition(conditions.TypeDependenciesMet, false, conditions.ReasonWaitingForDependency, "One or more dependencies are not ready")
		cm.SetProgressing(conditions.ReasonDependenciesNotMet, conditions.ReasonWaitingForDependency, "Waiting for dependencies")
		return r.updateStatus(ctx, addon, cm)
	}
	cm.SetOperationalCondition(conditions.TypeDependenciesMet, true, "AllDependenciesMet", "All dependencies are satisfied")

	extractedValues, err := r.extractValuesSources(ctx, addon)
	if err != nil {
		logger.Error(nil, "Failed to extract values sources", "addon", addon.Name, "reason", err.Error())
		cm.SetOperationalCondition(conditions.TypeValuesResolved, false, conditions.ReasonValueSourceError, err.Error())
		cm.SetDegraded(conditions.ReasonValuesNotResolved, conditions.ReasonValueSourceError, "Failed to extract values from sources")
		return r.updateStatus(ctx, addon, cm)
	}

	aggregatedValues, err := r.aggregateValues(ctx, addon)
	if err != nil {
		logger.Error(nil, "Failed to aggregate values", "addon", addon.Name, "reason", err.Error())
		cm.SetOperationalCondition(conditions.TypeValuesResolved, false, conditions.ReasonValueSourceError, err.Error())
		cm.SetDegraded(conditions.ReasonValuesNotResolved, conditions.ReasonValueSourceError, "Failed to aggregate AddonValues")
		return r.updateStatus(ctx, addon, cm)
	}

	finalValues, err := r.applyTemplates(aggregatedValues, addon, extractedValues)
	if err != nil {
		logger.Error(nil, "Failed to apply templates", "addon", addon.Name, "reason", err.Error())
		cm.SetOperationalCondition(conditions.TypeValuesResolved, false, conditions.ReasonTemplateError, err.Error())
		cm.SetDegraded(conditions.ReasonValuesNotResolved, conditions.ReasonTemplateError, "Failed to render value templates")
		return r.updateStatus(ctx, addon, cm)
	}

	// Compute hash from final values (includes both aggregated values and extracted values after templates)
	r.computeValuesHash(ctx, addon, finalValues)
	cm.SetOperationalCondition(conditions.TypeValuesResolved, true, "ValuesResolved", "All values successfully resolved")

	if err := r.reconcileApplication(ctx, addon, finalValues); err != nil {
		logger.Error(nil, "Failed to reconcile Application", "addon", addon.Name, "reason", err.Error())
		cm.SetOperationalCondition(conditions.TypeApplicationCreated, false, conditions.ReasonApplicationError, err.Error())
		cm.SetDegraded(conditions.ReasonApplicationFailed, conditions.ReasonApplicationError, "Failed to create/update ArgoCD Application")
		return r.updateStatus(ctx, addon, cm)
	}
	cm.SetOperationalCondition(conditions.TypeApplicationCreated, true, "ApplicationCreated", "Argo CD Application created/updated")

	if err := r.translateApplicationStatus(ctx, addon, cm); err != nil {
		logger.Error(nil, "Failed to translate Application status", "addon", addon.Name, "reason", err.Error())
	}

	if cm.IsConditionTrue(conditions.TypeSynced) && cm.IsConditionTrue(conditions.TypeHealthy) {
		cm.SetReady(conditions.ReasonFullyReconciled, "Addon is fully reconciled and healthy")
	} else if !cm.IsDegraded() {
		reason := conditions.ReasonWaitingForSync
		message := "Waiting for Application to sync"
		if cm.IsConditionTrue(conditions.TypeSynced) {
			reason = conditions.ReasonWaitingForHealthy
			message = "Waiting for Application to become healthy"
		}
		cm.SetProgressing(conditions.ReasonNotSynced, reason, message)
	}

	return r.updateStatus(ctx, addon, cm)
}

func (r *AddonReconciler) reconcileDelete(ctx context.Context, addon *addonsv1alpha1.Addon) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling Addon deletion")

	// Cleanup dynamic watches for this Addon
	r.cleanupWatchesForAddon(addon)

	if controllerutil.ContainsFinalizer(addon, finalizerName) {
		// Explicitly delete ArgoCD Application for reliability
		// (OwnerReference may not work for cross-namespace resources)
		argoApp := &argocdv1alpha1.Application{}
		argoApp.Name = addon.Name
		argoApp.Namespace = addon.Spec.Backend.Namespace

		if err := r.Delete(ctx, argoApp); client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to delete ArgoCD Application", "name", argoApp.Name, "namespace", argoApp.Namespace)
			return ctrl.Result{}, err
		}
		logger.Info("Deleted ArgoCD Application", "name", argoApp.Name, "namespace", argoApp.Namespace)

		// Remove finalizer after cleanup
		addonKey := client.ObjectKeyFromObject(addon)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			fresh := &addonsv1alpha1.Addon{}
			if getErr := r.Get(ctx, addonKey, fresh); getErr != nil {
				return getErr
			}
			controllerutil.RemoveFinalizer(fresh, finalizerName)
			return r.Update(ctx, fresh)
		})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *AddonReconciler) ensureFinalizer(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	if !controllerutil.ContainsFinalizer(addon, finalizerName) {
		addonKey := client.ObjectKeyFromObject(addon)
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			fresh := &addonsv1alpha1.Addon{}
			if err := r.Get(ctx, addonKey, fresh); err != nil {
				return err
			}
			if controllerutil.ContainsFinalizer(fresh, finalizerName) {
				return nil
			}
			controllerutil.AddFinalizer(fresh, finalizerName)
			return r.Update(ctx, fresh)
		})
	}
	return nil
}

func (r *AddonReconciler) checkDependencies(ctx context.Context, addon *addonsv1alpha1.Addon) (bool, error) {
	checker := dependencies.NewDependencyChecker(r.Client)
	result, err := checker.CheckDependencies(ctx, addon, addon.Spec.Backend.Namespace)
	if err != nil {
		return false, err
	}
	return result.Satisfied, nil
}

func (r *AddonReconciler) aggregateValues(ctx context.Context, addon *addonsv1alpha1.Addon) (map[string]any, error) {
	aggregator := values.NewAggregator(r.Client)
	return aggregator.AggregateValues(ctx, addon)
}

// computeValuesHash computes and sets the hash of the final values.
// This is used to detect when values change and the Application needs updating.
func (r *AddonReconciler) computeValuesHash(ctx context.Context, addon *addonsv1alpha1.Addon, finalValues map[string]any) {
	logger := log.FromContext(ctx)
	hash, err := values.ComputeHash(finalValues)
	if err != nil {
		// Log error but continue - use empty hash which forces update on every reconcile
		// This is safe but suboptimal; the error indicates a serialization issue
		logger.Error(err, "Failed to compute values hash, change detection disabled")
		hash = ""
	}
	addon.Status.ValuesHash = hash
}

func (r *AddonReconciler) reconcileApplication(ctx context.Context, addon *addonsv1alpha1.Addon, helmValues map[string]any) error {
	logger := log.FromContext(ctx)
	builder := argocd.NewApplicationBuilder()
	appNamespace := addon.Spec.Backend.Namespace
	appKey := types.NamespacedName{Name: addon.Name, Namespace: appNamespace}

	existing := &argocdv1alpha1.Application{}
	err := r.Get(ctx, appKey, existing)

	if apierrors.IsNotFound(err) {
		app, buildErr := builder.Build(addon, appNamespace, helmValues)
		if buildErr != nil {
			return buildErr
		}

		logger.Info("Creating Argo CD Application", "name", app.Name, "namespace", app.Namespace)
		if createErr := r.Create(ctx, app); createErr != nil {
			if apierrors.IsAlreadyExists(createErr) {
				return nil
			}
			r.Recorder.Eventf(addon, "Warning", "ApplicationCreateFailed",
				"Failed to create Application %s/%s: %v", app.Namespace, app.Name, createErr)
			return createErr
		}

		r.Recorder.Eventf(addon, "Normal", "ApplicationCreated",
			"Created Application %s/%s", app.Namespace, app.Name)

		addon.Status.ApplicationRef = builder.GetApplicationRef(addon, appNamespace)
		return nil
	}

	if err != nil {
		return err
	}

	needsUpdate, reason, checkErr := builder.NeedsUpdate(existing, addon, appNamespace, helmValues)
	if checkErr != nil {
		return checkErr
	}

	if !needsUpdate {
		addon.Status.ApplicationRef = builder.GetApplicationRef(addon, appNamespace)
		return nil
	}

	logger.Info("Application needs update", "reason", reason)

	updateErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh := &argocdv1alpha1.Application{}
		if getErr := r.Get(ctx, appKey, fresh); getErr != nil {
			return getErr
		}

		if specErr := builder.UpdateSpec(fresh, addon, appNamespace, helmValues); specErr != nil {
			return specErr
		}

		return r.Update(ctx, fresh)
	})

	if updateErr != nil {
		r.Recorder.Eventf(addon, "Warning", "ApplicationUpdateFailed",
			"Failed to update Application %s/%s: %v", appNamespace, addon.Name, updateErr)
		return updateErr
	}

	logger.Info("Updated Argo CD Application", "name", addon.Name, "namespace", appNamespace)
	r.Recorder.Eventf(addon, "Normal", "ApplicationUpdated",
		"Updated Application %s/%s", appNamespace, addon.Name)

	addon.Status.ApplicationRef = builder.GetApplicationRef(addon, appNamespace)
	return nil
}

func (r *AddonReconciler) translateApplicationStatus(ctx context.Context, addon *addonsv1alpha1.Addon, cm *conditions.Manager) error {
	app := &argocdv1alpha1.Application{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      addon.Name,
		Namespace: addon.Spec.Backend.Namespace,
	}, app); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	translator := status.NewStatusTranslator()
	translator.UpdateConditions(cm, app)

	return nil
}

func (r *AddonReconciler) updateStatus(ctx context.Context, addon *addonsv1alpha1.Addon, cm *conditions.Manager) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	addon.Status.ObservedGeneration = addon.Generation

	if err := r.Status().Update(ctx, addon); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict updating Addon status, will retry", "addon", addon.Name)
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to update Addon status")
		return ctrl.Result{}, err
	}

	if cm.IsDegraded() {
		return ctrl.Result{RequeueAfter: requeueIntervalDegraded}, nil
	}

	return ctrl.Result{}, nil
}

func (r *AddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	// Initialize dynamic watch components
	r.resolver = dynamicwatch.NewResolver()
	r.tracker = dynamicwatch.NewTracker()

	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&addonsv1alpha1.AddonValue{},
		addonLabelKey,
		func(obj client.Object) []string {
			av := obj.(*addonsv1alpha1.AddonValue)
			if addon, ok := av.Labels[addonLabelKey]; ok {
				return []string{addon}
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("create AddonValue index: %w", err)
	}

	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&addonsv1alpha1.Addon{},
		valuesSourcesIndexKey,
		func(obj client.Object) []string {
			addon := obj.(*addonsv1alpha1.Addon)
			if len(addon.Spec.ValuesSources) == 0 {
				return nil
			}

			keys := make([]string, 0, len(addon.Spec.ValuesSources))
			for _, source := range addon.Spec.ValuesSources {
				key := sources.SourceRefKey(source.SourceRef)
				keys = append(keys, key)
			}
			return keys
		},
	); err != nil {
		return fmt.Errorf("create valuesSources index: %w", err)
	}

	// Index for finding addons that depend on a given addon
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&addonsv1alpha1.Addon{},
		dependencyIndexKey,
		func(obj client.Object) []string {
			addon := obj.(*addonsv1alpha1.Addon)
			if len(addon.Spec.InitDependencies) == 0 {
				return nil
			}

			deps := make([]string, 0, len(addon.Spec.InitDependencies))
			for _, dep := range addon.Spec.InitDependencies {
				deps = append(deps, dep.Name)
			}
			return deps
		},
	); err != nil {
		return fmt.Errorf("create initDependencies index: %w", err)
	}

	// Build controller first to get controller reference for dynamic watches
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.Addon{}).
		Owns(&argocdv1alpha1.Application{}).
		Watches(
			&addonsv1alpha1.AddonValue{},
			handler.EnqueueRequestsFromMapFunc(r.findAddonsForAddonValue),
		).
		Watches(
			&addonsv1alpha1.Addon{},
			handler.EnqueueRequestsFromMapFunc(r.findDependentAddons),
		).
		Named("addon").
		Build(r)
	if err != nil {
		return fmt.Errorf("build controller: %w", err)
	}

	// Initialize watch manager with controller reference
	r.watchManager = dynamicwatch.NewWatchManager(
		mgr.GetCache(),
		controller,
		r.findAddonsBySourceRefUnstructured,
		mgr.GetLogger().WithName("addon-controller"),
	)

	// Bootstrap watches for core types (Secret, ConfigMap)
	// These are always needed and should be available
	coreTypes := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
	}
	for _, gvk := range coreTypes {
		if err := r.watchManager.EnsureWatch(ctx, gvk); err != nil {
			return fmt.Errorf("bootstrap watch for %s: %w", gvk.String(), err)
		}
	}

	return nil
}

func (r *AddonReconciler) findAddonsForAddonValue(ctx context.Context, obj client.Object) []reconcile.Request {
	av, ok := obj.(*addonsv1alpha1.AddonValue)
	if !ok {
		return nil
	}

	var addons addonsv1alpha1.AddonList
	if err := r.List(ctx, &addons); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, addon := range addons.Items {
		if addonMatchesValue(&addon, av) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: addon.Name},
			})
		}
	}
	return requests
}

// addonMatchesValue checks if any selector of Addon exact-matches AddonValue labels.
func addonMatchesValue(addon *addonsv1alpha1.Addon, av *addonsv1alpha1.AddonValue) bool {
	allSelectors := collectAllSelectors(addon)
	for _, sel := range allSelectors {
		if values.ExactMatchAddonLabels(sel.MatchLabels, av.Labels) {
			return true
		}
	}
	return false
}

// collectAllSelectors combines spec and status selectors.
func collectAllSelectors(addon *addonsv1alpha1.Addon) []addonsv1alpha1.ValuesSelector {
	result := make([]addonsv1alpha1.ValuesSelector, 0,
		len(addon.Spec.ValuesSelectors)+len(addon.Status.PhaseValuesSelector))
	result = append(result, addon.Spec.ValuesSelectors...)
	result = append(result, addon.Status.PhaseValuesSelector...)
	return result
}

func (r *AddonReconciler) findDependentAddons(ctx context.Context, obj client.Object) []reconcile.Request {
	changedAddon, ok := obj.(*addonsv1alpha1.Addon)
	if !ok {
		return nil
	}

	// Use index to find addons that depend on the changed addon
	var dependentAddons addonsv1alpha1.AddonList
	if err := r.List(ctx, &dependentAddons, client.MatchingFields{
		dependencyIndexKey: changedAddon.Name,
	}); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(dependentAddons.Items))
	for _, addon := range dependentAddons.Items {
		// Skip self (shouldn't happen with proper index, but be defensive)
		if addon.Name == changedAddon.Name {
			continue
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: addon.Name,
			},
		})
	}

	return requests
}

func (r *AddonReconciler) extractValuesSources(ctx context.Context, addon *addonsv1alpha1.Addon) (map[string]any, error) {
	if len(addon.Spec.ValuesSources) == 0 {
		return make(map[string]any), nil
	}

	extractor := sources.NewExtractor(r.Client)
	return extractor.Extract(ctx, addon.Spec.ValuesSources)
}

func (r *AddonReconciler) applyTemplates(aggregatedValues map[string]any, addon *addonsv1alpha1.Addon, extractedValues map[string]any) (map[string]any, error) {
	ctx := sources.TemplateContext{
		Variables: addon.Spec.Variables,
		Values:    extractedValues,
	}

	engine := sources.NewTemplateEngine()
	return engine.Apply(aggregatedValues, ctx)
}

// updateDynamicWatches updates watches based on the current valuesSources.
// It ensures watches exist for all GVKs referenced in the Addon's valuesSources
// and releases watches for GVKs that are no longer needed.
// It also retries any pending watches (CRDs that weren't available before).
func (r *AddonReconciler) updateDynamicWatches(ctx context.Context, addon *addonsv1alpha1.Addon) error {
	logger := log.FromContext(ctx)

	// Skip if dynamic watch components are not initialized
	if r.watchManager == nil || r.tracker == nil || r.resolver == nil {
		return nil
	}

	// Extract current GVKs from valuesSources
	currentGVKs := r.extractGVKsFromValuesSources(addon)

	// Update tracker and get diff
	added, removed := r.tracker.SetGVKs(addon.UID, currentGVKs)

	// Ensure watches for added GVKs
	for gvk := range added {
		if err := r.watchManager.EnsureWatch(ctx, gvk); err != nil {
			return fmt.Errorf("ensure watch for %s: %w", gvk.String(), err)
		}
	}

	// Release watches for removed GVKs
	for gvk := range removed {
		r.watchManager.ReleaseWatch(gvk)
	}

	// Retry pending watches (CRDs that weren't available before)
	pendingGVKs := r.watchManager.GetPendingGVKs()
	for _, gvk := range pendingGVKs {
		if err := r.watchManager.RetryPendingWatch(ctx, gvk); err != nil {
			// Log but continue - will retry on next reconcile
			logger.V(1).Info("pending watch still unavailable",
				"gvk", gvk.String(),
				"error", err.Error())
		} else {
			// Watch activated - record event
			r.Recorder.Eventf(addon, "Normal", "WatchActivated",
				"Watch activated for %s", gvk.String())
		}
	}

	return nil
}

// extractGVKsFromValuesSources extracts all GVKs from the Addon's valuesSources.
func (r *AddonReconciler) extractGVKsFromValuesSources(addon *addonsv1alpha1.Addon) dynamicwatch.GVKSet {
	gvks := make(dynamicwatch.GVKSet)

	for _, source := range addon.Spec.ValuesSources {
		gvk, err := r.resolver.Resolve(source.SourceRef)
		if err != nil {
			// Log but continue - invalid refs are handled elsewhere
			continue
		}
		gvks[gvk] = struct{}{}
	}

	return gvks
}

// cleanupWatchesForAddon releases watches when an Addon is deleted.
func (r *AddonReconciler) cleanupWatchesForAddon(addon *addonsv1alpha1.Addon) {
	// Skip if dynamic watch components are not initialized
	if r.watchManager == nil || r.tracker == nil {
		return
	}

	// Remove tracking for this Addon and get GVKs that were tracked
	removedGVKs := r.tracker.RemoveAddon(addon.UID)

	// Release watches for all GVKs
	for gvk := range removedGVKs {
		r.watchManager.ReleaseWatch(gvk)
	}
}

// findAddonsBySourceRefUnstructured is a typed map function for Unstructured objects.
// It's used by the DynamicWatchManager for dynamic source watches.
func (r *AddonReconciler) findAddonsBySourceRefUnstructured(ctx context.Context, obj *unstructured.Unstructured) []reconcile.Request {
	gvk := obj.GetObjectKind().GroupVersionKind()
	apiVersion := gvk.GroupVersion().String()
	kind := gvk.Kind

	refKey := fmt.Sprintf("%s/%s/%s/%s", apiVersion, kind, obj.GetNamespace(), obj.GetName())

	var addons addonsv1alpha1.AddonList
	if err := r.List(ctx, &addons, client.MatchingFields{
		valuesSourcesIndexKey: refKey,
	}); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(addons.Items))
	for _, addon := range addons.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: addon.Name},
		})
	}

	return requests
}
