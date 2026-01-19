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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

const addonLabelKey = "addons.in-cloud.io/addon"

var addonvaluelog = logf.Log.WithName("addonvalue-resource")

// SetupAddonValueWebhookWithManager registers the webhook for AddonValue in the manager.
func SetupAddonValueWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&addonsv1alpha1.AddonValue{}).
		WithValidator(&AddonValueCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-addons-in-cloud-io-v1alpha1-addonvalue,mutating=false,failurePolicy=fail,sideEffects=None,groups=addons.in-cloud.io,resources=addonvalues,verbs=create;update,versions=v1alpha1,name=vaddonvalue-v1alpha1.kb.io,admissionReviewVersions=v1

// AddonValueCustomValidator validates the AddonValue resource.
type AddonValueCustomValidator struct{}

var _ webhook.CustomValidator = &AddonValueCustomValidator{}

// ValidateCreate validates AddonValue on creation.
func (v *AddonValueCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addonvalue, ok := obj.(*addonsv1alpha1.AddonValue)
	if !ok {
		return nil, fmt.Errorf("expected an AddonValue object but got %T", obj)
	}
	addonvaluelog.Info("Validation for AddonValue upon creation", "name", addonvalue.GetName())

	return validateAddonValue(addonvalue), nil
}

// ValidateUpdate validates AddonValue on update.
func (v *AddonValueCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	addonvalue, ok := newObj.(*addonsv1alpha1.AddonValue)
	if !ok {
		return nil, fmt.Errorf("expected an AddonValue object for the newObj but got %T", newObj)
	}
	addonvaluelog.Info("Validation for AddonValue upon update", "name", addonvalue.GetName())

	return validateAddonValue(addonvalue), nil
}

// ValidateDelete validates AddonValue on deletion.
func (v *AddonValueCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addonvalue, ok := obj.(*addonsv1alpha1.AddonValue)
	if !ok {
		return nil, fmt.Errorf("expected an AddonValue object but got %T", obj)
	}
	addonvaluelog.Info("Validation for AddonValue upon deletion", "name", addonvalue.GetName())

	return nil, nil
}

// validateAddonValue checks AddonValue and returns warnings (not errors).
func validateAddonValue(addonvalue *addonsv1alpha1.AddonValue) admission.Warnings {
	var warnings admission.Warnings

	if addonvalue.Labels == nil {
		warnings = append(warnings, "missing labels for matching: AddonValue without labels won't be selected by any Addon")
		return warnings
	}

	if _, ok := addonvalue.Labels[addonLabelKey]; !ok {
		warnings = append(warnings, fmt.Sprintf("missing label %s: this label is recommended for matching", addonLabelKey))
	}

	return warnings
}
