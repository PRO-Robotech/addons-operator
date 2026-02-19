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

var addonclaimlog = logf.Log.WithName("addonclaim-resource")

// SetupAddonClaimWebhookWithManager registers the webhook for AddonClaim in the manager.
func SetupAddonClaimWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&addonsv1alpha1.AddonClaim{}).
		WithValidator(&AddonClaimCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-addons-in-cloud-io-v1alpha1-addonclaim,mutating=false,failurePolicy=fail,sideEffects=None,groups=addons.in-cloud.io,resources=addonclaims,verbs=create;update,versions=v1alpha1,name=vaddonclaim-v1alpha1.kb.io,admissionReviewVersions=v1

// AddonClaimCustomValidator validates the AddonClaim resource.
type AddonClaimCustomValidator struct{}

var _ webhook.CustomValidator = &AddonClaimCustomValidator{}

// ValidateCreate validates AddonClaim on creation.
func (v *AddonClaimCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addonclaim, ok := obj.(*addonsv1alpha1.AddonClaim)
	if !ok {
		return nil, fmt.Errorf("expected an AddonClaim object but got %T", obj)
	}
	addonclaimlog.Info("Validation for AddonClaim upon creation", "name", addonclaim.GetName())

	return nil, validateAddonClaim(addonclaim)
}

// ValidateUpdate validates AddonClaim on update.
func (v *AddonClaimCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	addonclaim, ok := newObj.(*addonsv1alpha1.AddonClaim)
	if !ok {
		return nil, fmt.Errorf("expected an AddonClaim object for the newObj but got %T", newObj)
	}
	addonclaimlog.Info("Validation for AddonClaim upon update", "name", addonclaim.GetName())

	return nil, validateAddonClaim(addonclaim)
}

// ValidateDelete validates AddonClaim on deletion.
func (v *AddonClaimCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addonclaim, ok := obj.(*addonsv1alpha1.AddonClaim)
	if !ok {
		return nil, fmt.Errorf("expected an AddonClaim object but got %T", obj)
	}
	addonclaimlog.Info("Validation for AddonClaim upon deletion", "name", addonclaim.GetName())

	return nil, nil
}

// validateAddonClaim performs validation rules for AddonClaim.
func validateAddonClaim(addonclaim *addonsv1alpha1.AddonClaim) error {
	// Validate values/valuesString mutual exclusivity
	if addonclaim.Spec.Values != nil && len(addonclaim.Spec.Values.Raw) > 0 && addonclaim.Spec.ValuesString != "" {
		return errors.New("values and valuesString are mutually exclusive")
	}

	return nil
}
