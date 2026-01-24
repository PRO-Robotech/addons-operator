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

package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var addonphaselog = logf.Log.WithName("addonphase-resource")

// SetupAddonPhaseWebhookWithManager registers the webhook for AddonPhase in the manager.
func SetupAddonPhaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&addonsv1alpha1.AddonPhase{}).
		WithValidator(&AddonPhaseCustomValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-addons-in-cloud-io-v1alpha1-addonphase,mutating=false,failurePolicy=fail,sideEffects=None,groups=addons.in-cloud.io,resources=addonphases,verbs=create;update,versions=v1alpha1,name=vaddonphase-v1alpha1.kb.io,admissionReviewVersions=v1

// AddonPhaseCustomValidator validates the AddonPhase resource.
// It requires a client to check that the referenced Addon exists.
type AddonPhaseCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &AddonPhaseCustomValidator{}

// ValidateCreate validates AddonPhase on creation.
func (v *AddonPhaseCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	addonphase, ok := obj.(*addonsv1alpha1.AddonPhase)
	if !ok {
		return nil, fmt.Errorf("expected an AddonPhase object but got %T", obj)
	}
	addonphaselog.Info("Validation for AddonPhase upon creation", "name", addonphase.GetName())

	return nil, v.validateAddonPhase(ctx, addonphase)
}

// ValidateUpdate validates AddonPhase on update.
func (v *AddonPhaseCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	addonphase, ok := newObj.(*addonsv1alpha1.AddonPhase)
	if !ok {
		return nil, fmt.Errorf("expected an AddonPhase object for the newObj but got %T", newObj)
	}
	addonphaselog.Info("Validation for AddonPhase upon update", "name", addonphase.GetName())

	// Skip validation during deletion (finalizer removal)
	// The Addon may already be deleted at this point
	if !addonphase.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	return nil, v.validateAddonPhase(ctx, addonphase)
}

// ValidateDelete validates AddonPhase on deletion.
func (v *AddonPhaseCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addonphase, ok := obj.(*addonsv1alpha1.AddonPhase)
	if !ok {
		return nil, fmt.Errorf("expected an AddonPhase object but got %T", obj)
	}
	addonphaselog.Info("Validation for AddonPhase upon deletion", "name", addonphase.GetName())

	return nil, nil
}

// validateAddonPhase performs validation rules for AddonPhase.
func (v *AddonPhaseCustomValidator) validateAddonPhase(ctx context.Context, addonphase *addonsv1alpha1.AddonPhase) error {
	// Validate that Addon with the same name exists (1:1 relationship)
	addon := &addonsv1alpha1.Addon{}
	err := v.Client.Get(ctx, types.NamespacedName{Name: addonphase.Name}, addon)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("addon %s not found: AddonPhase must have a corresponding Addon with the same name", addonphase.Name)
		}
		return fmt.Errorf("failed to check addon existence: %w", err)
	}

	// Validate unique rule names
	ruleNames := make(map[string]struct{})
	for _, rule := range addonphase.Spec.Rules {
		if _, exists := ruleNames[rule.Name]; exists {
			return fmt.Errorf("duplicate rule name: %s", rule.Name)
		}
		ruleNames[rule.Name] = struct{}{}
	}

	return nil
}
