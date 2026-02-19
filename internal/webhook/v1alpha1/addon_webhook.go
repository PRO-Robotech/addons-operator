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
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

const supportedBackendType = "argocd"

var addonlog = logf.Log.WithName("addon-resource")

// SetupAddonWebhookWithManager registers the webhook for Addon in the manager.
func SetupAddonWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&addonsv1alpha1.Addon{}).
		WithValidator(&AddonCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-addons-in-cloud-io-v1alpha1-addon,mutating=false,failurePolicy=fail,sideEffects=None,groups=addons.in-cloud.io,resources=addons,verbs=create;update,versions=v1alpha1,name=vaddon-v1alpha1.kb.io,admissionReviewVersions=v1

// AddonCustomValidator validates the Addon resource.
type AddonCustomValidator struct{}

var _ webhook.CustomValidator = &AddonCustomValidator{}

// ValidateCreate validates Addon on creation.
func (v *AddonCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addon, ok := obj.(*addonsv1alpha1.Addon)
	if !ok {
		return nil, fmt.Errorf("expected an Addon object but got %T", obj)
	}
	addonlog.Info("Validation for Addon upon creation", "name", addon.GetName())

	return nil, validateAddon(addon)
}

// ValidateUpdate validates Addon on update.
func (v *AddonCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	addon, ok := newObj.(*addonsv1alpha1.Addon)
	if !ok {
		return nil, fmt.Errorf("expected an Addon object for the newObj but got %T", newObj)
	}
	addonlog.Info("Validation for Addon upon update", "name", addon.GetName())

	return nil, validateAddon(addon)
}

// ValidateDelete validates Addon on deletion.
func (v *AddonCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addon, ok := obj.(*addonsv1alpha1.Addon)
	if !ok {
		return nil, fmt.Errorf("expected an Addon object but got %T", obj)
	}
	addonlog.Info("Validation for Addon upon deletion", "name", addon.GetName())

	return nil, nil
}

// validateAddon performs validation rules for Addon.
func validateAddon(addon *addonsv1alpha1.Addon) error {
	// Validate chart/path mutual exclusivity
	if addon.Spec.Chart == "" && addon.Spec.Path == "" {
		return errors.New("either chart or path must be specified")
	}
	if addon.Spec.Chart != "" && addon.Spec.Path != "" {
		return errors.New("chart and path are mutually exclusive")
	}

	// Validate backend type
	if addon.Spec.Backend.Type != supportedBackendType {
		return fmt.Errorf("unsupported backend type: %s, only '%s' is supported", addon.Spec.Backend.Type, supportedBackendType)
	}

	// Validate unique selector names
	selectorNames := make(map[string]struct{})
	for _, sel := range addon.Spec.ValuesSelectors {
		if _, exists := selectorNames[sel.Name]; exists {
			return fmt.Errorf("duplicate selector name: %s", sel.Name)
		}
		selectorNames[sel.Name] = struct{}{}
	}

	// Validate initDependencies criteria
	for i, dep := range addon.Spec.InitDependencies {
		if err := validateCriteria(dep.Criteria, fmt.Sprintf("spec.initDependencies[%d].criteria", i)); err != nil {
			return err
		}
	}

	return nil
}
