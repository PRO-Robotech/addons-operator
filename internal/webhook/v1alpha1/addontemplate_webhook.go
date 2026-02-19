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
	"addons-operator/internal/controller/addonclaim"
)

var addontemplatelog = logf.Log.WithName("addontemplate-resource")

// SetupAddonTemplateWebhookWithManager registers the webhook for AddonTemplate in the manager.
func SetupAddonTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&addonsv1alpha1.AddonTemplate{}).
		WithValidator(&AddonTemplateCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-addons-in-cloud-io-v1alpha1-addontemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=addons.in-cloud.io,resources=addontemplates,verbs=create;update,versions=v1alpha1,name=vaddontemplate-v1alpha1.kb.io,admissionReviewVersions=v1

// AddonTemplateCustomValidator validates the AddonTemplate resource.
type AddonTemplateCustomValidator struct{}

var _ webhook.CustomValidator = &AddonTemplateCustomValidator{}

// ValidateCreate validates AddonTemplate on creation.
func (v *AddonTemplateCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addontemplate, ok := obj.(*addonsv1alpha1.AddonTemplate)
	if !ok {
		return nil, fmt.Errorf("expected an AddonTemplate object but got %T", obj)
	}
	addontemplatelog.Info("Validation for AddonTemplate upon creation", "name", addontemplate.GetName())

	return nil, validateAddonTemplate(addontemplate)
}

// ValidateUpdate validates AddonTemplate on update.
func (v *AddonTemplateCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	addontemplate, ok := newObj.(*addonsv1alpha1.AddonTemplate)
	if !ok {
		return nil, fmt.Errorf("expected an AddonTemplate object for the newObj but got %T", newObj)
	}
	addontemplatelog.Info("Validation for AddonTemplate upon update", "name", addontemplate.GetName())

	return nil, validateAddonTemplate(addontemplate)
}

// ValidateDelete validates AddonTemplate on deletion.
func (v *AddonTemplateCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	addontemplate, ok := obj.(*addonsv1alpha1.AddonTemplate)
	if !ok {
		return nil, fmt.Errorf("expected an AddonTemplate object but got %T", obj)
	}
	addontemplatelog.Info("Validation for AddonTemplate upon deletion", "name", addontemplate.GetName())

	return nil, nil
}

// validateAddonTemplate performs validation rules for AddonTemplate.
func validateAddonTemplate(addontemplate *addonsv1alpha1.AddonTemplate) error {
	if addontemplate.Spec.Template == "" {
		return errors.New("spec.template must not be empty")
	}

	if err := addonclaim.ParseTemplate(addontemplate.Spec.Template); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	return nil
}
