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

package addonclaim

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"sigs.k8s.io/yaml"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/addonclaim/remoteclient"
	pkgconditions "addons-operator/pkg/conditions"
)

const (
	DefaultPollingInterval  = 15 * time.Second
	requeueIntervalDegraded = 60 * time.Second
	finalizerName           = "addons.in-cloud.io/addonclaim-finalizer"
	secretKubeconfigKey     = "value"
	addonLabelKey           = "addons.in-cloud.io/addon"
	valuesLabelKey          = "addons.in-cloud.io/values"
	valuesLabelValue        = "claim"

	// Condition types specific to AddonClaim.
	TypeTemplateRendered = "TemplateRendered"
	TypeRemoteConnected  = "RemoteConnected"
	TypeAddonSynced      = "AddonSynced"

	// Condition reasons specific to AddonClaim.
	ReasonTemplateNotFound    = "TemplateNotFound"
	ReasonTemplateRenderFail  = "TemplateRenderFailed"
	ReasonSecretNotFound      = "SecretNotFound"
	ReasonSecretInvalid       = "SecretInvalid"
	ReasonRemoteClientFail    = "RemoteClientFailed"
	ReasonRemoteOperationFail = "RemoteOperationFailed"
	ReasonAddonNotReady       = "AddonNotReady"
)

// Reconciler reconciles AddonClaim objects.
type Reconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	RemoteClients   *remoteclient.Cache
	Renderer        *Renderer
	PollingInterval time.Duration
}

// reconcileContext holds intermediate state passed between reconciliation steps.
type reconcileContext struct {
	claim        *addonsv1alpha1.AddonClaim
	cm           *pkgconditions.Manager
	addon        *addonsv1alpha1.Addon
	remoteClient client.Client
	oldStatus    addonsv1alpha1.AddonClaimStatus
}

// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addonclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addonclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addonclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=addons.in-cloud.io,resources=addontemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	claim := &addonsv1alpha1.AddonClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	cm := pkgconditions.New(&claim.Status.Conditions, claim.Generation)
	cm.EnsureAllConditions()

	logger.V(1).Info("Reconciling AddonClaim", "templateRef", claim.Spec.TemplateRef.Name)

	if !claim.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, claim)
	}

	if err := r.ensureFinalizer(ctx, claim); err != nil {
		return ctrl.Result{}, err
	}

	rctx := &reconcileContext{
		claim:     claim,
		cm:        cm,
		oldStatus: *claim.Status.DeepCopy(),
	}

	if result, err, done := r.renderAddon(ctx, rctx); done {
		return result, err
	}

	if result, err, done := r.connectRemote(ctx, rctx); done {
		return result, err
	}

	if result, err, done := r.syncRemoteResources(ctx, rctx); done {
		return result, err
	}

	return r.determineRequeue(ctx, rctx)
}

// renderAddon fetches the AddonTemplate, renders it with the claim context,
// and overrides the rendered name with spec.addon.name.
func (r *Reconciler) renderAddon(ctx context.Context, rctx *reconcileContext) (ctrl.Result, error, bool) {
	claim := rctx.claim
	cm := rctx.cm

	tmpl := &addonsv1alpha1.AddonTemplate{}
	if err := r.Get(ctx, types.NamespacedName{Name: claim.Spec.TemplateRef.Name}, tmpl); err != nil {
		if apierrors.IsNotFound(err) {
			cm.SetCondition(TypeTemplateRendered, false, ReasonTemplateNotFound,
				fmt.Sprintf("AddonTemplate %q not found", claim.Spec.TemplateRef.Name))
			cm.SetDegraded(ReasonTemplateNotFound, ReasonTemplateNotFound, "AddonTemplate not found")
			result, updateErr := r.updateStatus(ctx, rctx)

			return result, updateErr, true
		}

		return ctrl.Result{}, err, true
	}

	addon, err := r.Renderer.Render(tmpl.Spec.Template, claim)
	if err != nil {
		cm.SetCondition(TypeTemplateRendered, false, ReasonTemplateRenderFail, err.Error())
		cm.SetDegraded(ReasonTemplateRenderFail, ReasonTemplateRenderFail, "Failed to render AddonTemplate")
		result, updateErr := r.updateStatus(ctx, rctx)

		return result, updateErr, true
	}

	cm.SetCondition(TypeTemplateRendered, true, "Rendered", "Template rendered successfully")

	// Override rendered name with the explicit identity from spec.
	addon.Name = claim.Spec.Addon.Name
	rctx.addon = addon

	return ctrl.Result{}, nil, false
}

// connectRemote fetches the kubeconfig Secret and obtains a remote client.
func (r *Reconciler) connectRemote(ctx context.Context, rctx *reconcileContext) (ctrl.Result, error, bool) {
	claim := rctx.claim
	cm := rctx.cm

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: claim.Namespace,
		Name:      claim.Spec.CredentialRef.Name,
	}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		if apierrors.IsNotFound(err) {
			cm.SetCondition(TypeRemoteConnected, false, ReasonSecretNotFound,
				fmt.Sprintf("Secret %q not found", claim.Spec.CredentialRef.Name))
			cm.SetDegraded(ReasonSecretNotFound, ReasonSecretNotFound, "Credential Secret not found")
			result, updateErr := r.updateStatus(ctx, rctx)

			return result, updateErr, true
		}

		return ctrl.Result{}, err, true
	}

	kubeconfigData, ok := secret.Data[secretKubeconfigKey]
	if !ok || len(kubeconfigData) == 0 {
		cm.SetCondition(TypeRemoteConnected, false, ReasonSecretInvalid,
			fmt.Sprintf("Secret %q missing key %q", secret.Name, secretKubeconfigKey))
		cm.SetDegraded(ReasonSecretInvalid, ReasonSecretInvalid, "Credential Secret invalid")
		result, updateErr := r.updateStatus(ctx, rctx)

		return result, updateErr, true
	}

	cacheKey := remoteclient.CacheKey{
		Namespace:  claim.Namespace,
		SecretName: claim.Spec.CredentialRef.Name,
	}
	rc, err := r.RemoteClients.GetOrCreate(kubeconfigData, cacheKey, secret.ResourceVersion)
	if err != nil {
		cm.SetCondition(TypeRemoteConnected, false, ReasonRemoteClientFail, err.Error())
		cm.SetDegraded(ReasonRemoteClientFail, ReasonRemoteClientFail, "Failed to create remote client")
		result, updateErr := r.updateStatus(ctx, rctx)

		return result, updateErr, true
	}

	cm.SetCondition(TypeRemoteConnected, true, "Connected", "Remote cluster connection established")
	rctx.remoteClient = rc

	return ctrl.Result{}, nil, false
}

// syncRemoteResources creates/updates AddonValue first, then Addon in the infra cluster,
// and syncs the remote Addon status back.
func (r *Reconciler) syncRemoteResources(ctx context.Context, rctx *reconcileContext) (ctrl.Result, error, bool) {
	claim := rctx.claim
	cm := rctx.cm
	rc := rctx.remoteClient
	addon := rctx.addon

	if err := r.reconcileRemoteAddonValue(ctx, rc, claim, addon.Name); err != nil {
		cm.SetCondition(TypeAddonSynced, false, ReasonRemoteOperationFail, err.Error())
		cm.SetDegraded(ReasonRemoteOperationFail, ReasonRemoteOperationFail, "Failed to sync AddonValue to remote cluster")
		r.Recorder.Eventf(claim, "Warning", "AddonValueSyncFailed", "Failed to sync AddonValue: %v", err)
		result, updateErr := r.updateStatus(ctx, rctx)

		return result, updateErr, true
	}

	if err := r.reconcileRemoteAddon(ctx, rc, addon); err != nil {
		cm.SetCondition(TypeAddonSynced, false, ReasonRemoteOperationFail, err.Error())
		cm.SetDegraded(ReasonRemoteOperationFail, ReasonRemoteOperationFail, "Failed to sync Addon to remote cluster")
		r.Recorder.Eventf(claim, "Warning", "AddonSyncFailed", "Failed to sync Addon: %v", err)
		result, updateErr := r.updateStatus(ctx, rctx)

		return result, updateErr, true
	}

	cm.SetCondition(TypeAddonSynced, true, "Synced", "Addon and AddonValue synced to remote cluster")
	r.syncRemoteAddonStatus(ctx, rc, claim, addon.Name)
	r.syncExternalStatus(claim)

	return ctrl.Result{}, nil, false
}

// determineRequeue updates status and decides whether to requeue based on remote Addon readiness.
func (r *Reconciler) determineRequeue(ctx context.Context, rctx *reconcileContext) (ctrl.Result, error) {
	claim := rctx.claim
	cm := rctx.cm

	r.mirrorDeployedCondition(claim, cm)

	if claim.Status.Ready != nil && *claim.Status.Ready {
		cm.SetReady(pkgconditions.ReasonFullyReconciled, "Remote Addon is ready")

		return r.updateStatus(ctx, rctx)
	}

	cm.SetProgressing(ReasonAddonNotReady, pkgconditions.ReasonReconciling, "Waiting for remote Addon to become ready")

	return r.updateStatusAndRequeue(ctx, rctx, r.pollingInterval())
}

func (r *Reconciler) reconcileDelete(ctx context.Context, claim *addonsv1alpha1.AddonClaim) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling AddonClaim deletion")

	if !controllerutil.ContainsFinalizer(claim, finalizerName) {
		return ctrl.Result{}, nil
	}

	if err := r.deleteRemoteResources(ctx, claim); err != nil {
		logger.Error(err, "Failed to delete remote resources, proceeding with finalizer removal")
		r.Recorder.Eventf(claim, "Warning", "RemoteCleanupFailed",
			"Failed to clean up remote resources: %v", err)
	}

	claimKey := client.ObjectKeyFromObject(claim)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh := &addonsv1alpha1.AddonClaim{}
		if getErr := r.Get(ctx, claimKey, fresh); getErr != nil {
			return getErr
		}
		controllerutil.RemoveFinalizer(fresh, finalizerName)

		return r.Update(ctx, fresh)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) deleteRemoteResources(ctx context.Context, claim *addonsv1alpha1.AddonClaim) error {
	addonName := claim.Spec.Addon.Name
	if addonName == "" {
		return nil
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: claim.Namespace,
		Name:      claim.Spec.CredentialRef.Name,
	}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		return fmt.Errorf("get credential secret: %w", err)
	}

	kubeconfigData, ok := secret.Data[secretKubeconfigKey]
	if !ok {
		return fmt.Errorf("secret %q missing key %q", secret.Name, secretKubeconfigKey)
	}

	cacheKey := remoteclient.CacheKey{
		Namespace:  claim.Namespace,
		SecretName: claim.Spec.CredentialRef.Name,
	}
	rc, err := r.RemoteClients.GetOrCreate(kubeconfigData, cacheKey, secret.ResourceVersion)
	if err != nil {
		return fmt.Errorf("create remote client: %w", err)
	}

	label := resolveValuesLabel(claim.Spec.ValueLabels)

	return r.deleteRemoteResourcesByName(ctx, rc, addonName, label)
}

func (r *Reconciler) deleteRemoteResourcesByName(ctx context.Context, rc client.Client, addonName, valuesLabel string) error {
	remoteAddon := &addonsv1alpha1.Addon{}
	remoteAddon.Name = addonName
	if err := rc.Delete(ctx, remoteAddon); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete remote Addon: %w", err)
	}

	avName := buildAddonValueName(addonName, valuesLabel)
	remoteValue := &addonsv1alpha1.AddonValue{}
	remoteValue.Name = avName
	if err := rc.Delete(ctx, remoteValue); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete remote AddonValue: %w", err)
	}

	return nil
}

func resolveValuesLabel(label string) string {
	if label == "" {
		return valuesLabelValue
	}

	return label
}

func buildAddonValueName(addonName, valuesLabel string) string {
	return addonName + "-" + valuesLabel + "-values"
}

func (r *Reconciler) ensureFinalizer(ctx context.Context, claim *addonsv1alpha1.AddonClaim) error {
	if !controllerutil.ContainsFinalizer(claim, finalizerName) {
		claimKey := client.ObjectKeyFromObject(claim)

		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			fresh := &addonsv1alpha1.AddonClaim{}
			if err := r.Get(ctx, claimKey, fresh); err != nil {
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

func (r *Reconciler) reconcileRemoteAddon(ctx context.Context, rc client.Client, desired *addonsv1alpha1.Addon) error {
	existing := &addonsv1alpha1.Addon{}
	err := rc.Get(ctx, types.NamespacedName{Name: desired.Name}, existing)

	if apierrors.IsNotFound(err) {
		return rc.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get remote Addon: %w", err)
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations

	return rc.Update(ctx, existing)
}

func (r *Reconciler) reconcileRemoteAddonValue(ctx context.Context, rc client.Client, claim *addonsv1alpha1.AddonClaim, addonName string) error {
	valuesStr, err := r.resolveValues(claim)
	if err != nil {
		return err
	}
	if valuesStr == "" {
		return nil
	}

	label := resolveValuesLabel(claim.Spec.ValueLabels)
	avName := buildAddonValueName(addonName, label)

	desired := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name: avName,
			Labels: map[string]string{
				addonLabelKey:  addonName,
				valuesLabelKey: label,
			},
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: valuesStr,
		},
	}

	existing := &addonsv1alpha1.AddonValue{}
	err = rc.Get(ctx, types.NamespacedName{Name: avName}, existing)

	if apierrors.IsNotFound(err) {
		return rc.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get remote AddonValue: %w", err)
	}

	existing.Labels = desired.Labels
	existing.Spec = desired.Spec

	return rc.Update(ctx, existing)
}

func (r *Reconciler) resolveValues(claim *addonsv1alpha1.AddonClaim) (string, error) {
	if claim.Spec.ValuesString != "" {
		return claim.Spec.ValuesString, nil
	}
	if claim.Spec.Values != nil && claim.Spec.Values.Raw != nil {
		var parsed map[string]any
		if err := json.Unmarshal(claim.Spec.Values.Raw, &parsed); err != nil {
			return "", fmt.Errorf("unmarshal values JSON: %w", err)
		}

		yamlBytes, err := yaml.Marshal(parsed)
		if err != nil {
			return "", fmt.Errorf("marshal values to YAML: %w", err)
		}

		return string(yamlBytes), nil
	}

	return "", nil
}

func (r *Reconciler) syncRemoteAddonStatus(ctx context.Context, rc client.Client, claim *addonsv1alpha1.AddonClaim, addonName string) {
	logger := log.FromContext(ctx)

	remoteAddon := &addonsv1alpha1.Addon{}
	if err := rc.Get(ctx, types.NamespacedName{Name: addonName}, remoteAddon); err != nil {
		logger.V(1).Info("Cannot read remote Addon status", "error", err.Error())

		return
	}

	ready := isAddonReady(remoteAddon)
	claim.Status.Ready = &ready
	claim.Status.Deployed = remoteAddon.Status.Deployed
	claim.Status.RemoteAddonStatus = &addonsv1alpha1.RemoteAddonStatus{
		Deployed:   remoteAddon.Status.Deployed,
		Conditions: remoteAddon.Status.Conditions,
	}
}

func (r *Reconciler) mirrorDeployedCondition(claim *addonsv1alpha1.AddonClaim, cm *pkgconditions.Manager) {
	if claim.Status.Deployed {
		cm.SetCondition("Deployed", true, "Deployed", "Remote Addon has been successfully deployed")
	}
}

func (r *Reconciler) syncExternalStatus(claim *addonsv1alpha1.AddonClaim) {
	annotationType := claim.Annotations["external-status/type"]
	if annotationType == "" {
		claim.Status.ExternalManagedControlPlane = nil
		claim.Status.Initialized = nil
		claim.Status.Initialization = nil
		claim.Status.Version = ""

		return
	}

	trueVal := true
	claim.Status.ExternalManagedControlPlane = &trueVal

	var initialized bool
	if claim.Status.RemoteAddonStatus != nil {
		initialized = claim.Status.RemoteAddonStatus.Deployed
	}

	claim.Status.Initialized = &initialized
	cpInitialized := initialized
	claim.Status.Initialization = &addonsv1alpha1.Initialization{
		ControlPlaneInitialized: &cpInitialized,
	}

	claim.Status.Version = claim.Spec.Version
}

func (r *Reconciler) updateStatus(ctx context.Context, rctx *reconcileContext) (ctrl.Result, error) {
	claim := rctx.claim
	claim.Status.ObservedGeneration = claim.Generation

	result := ctrl.Result{}
	if rctx.cm.IsDegraded() {
		result.RequeueAfter = requeueIntervalDegraded
	}

	if apiequality.Semantic.DeepEqual(&rctx.oldStatus, &claim.Status) {
		return result, nil
	}

	return r.doStatusUpdate(ctx, claim, result)
}

func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, rctx *reconcileContext, interval time.Duration) (ctrl.Result, error) {
	claim := rctx.claim
	claim.Status.ObservedGeneration = claim.Generation

	result := ctrl.Result{RequeueAfter: interval}

	if apiequality.Semantic.DeepEqual(&rctx.oldStatus, &claim.Status) {
		return result, nil
	}

	return r.doStatusUpdate(ctx, claim, result)
}

func (r *Reconciler) doStatusUpdate(ctx context.Context, claim *addonsv1alpha1.AddonClaim, result ctrl.Result) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if err := r.Status().Update(ctx, claim); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict updating AddonClaim status, will retry")

			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to update AddonClaim status")

		return ctrl.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) pollingInterval() time.Duration {
	if r.PollingInterval > 0 {
		return r.PollingInterval
	}

	return DefaultPollingInterval
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Renderer = NewRenderer()
	r.RemoteClients = remoteclient.NewCache(mgr.GetScheme())
	r.RemoteClients.Start()

	return ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.AddonClaim{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findClaimsForSecret),
		).
		Named("addonclaim").
		Complete(r)
}

func (r *Reconciler) findClaimsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	var claims addonsv1alpha1.AddonClaimList
	if err := r.List(ctx, &claims, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, claim := range claims.Items {
		if claim.Spec.CredentialRef.Name == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: claim.Namespace,
					Name:      claim.Name,
				},
			})
		}
	}

	return requests
}

func isAddonReady(addon *addonsv1alpha1.Addon) bool {
	for _, cond := range addon.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}
