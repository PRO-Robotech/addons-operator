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
	"fmt"
	"maps"
	"sync/atomic"
	"time"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var testCounter int64

// uniqueName generates a unique name for test resources.
func uniqueName(base string) string {
	counter := atomic.AddInt64(&testCounter, 1)

	return fmt.Sprintf("%s-%d-%d", base, time.Now().UnixNano()%10000, counter)
}

// AddonOption is a function that modifies an Addon.
type AddonOption func(*addonsv1alpha1.Addon)

// WithValuesSelectors sets the valuesSelectors on the Addon.
func WithValuesSelectors(selectors []addonsv1alpha1.ValuesSelector) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.ValuesSelectors = selectors
	}
}

// WithValuesSources sets the valuesSources on the Addon.
func WithValuesSources(sources []addonsv1alpha1.ValueSource) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.ValuesSources = sources
	}
}

// WithVariables sets the variables on the Addon.
func WithVariables(vars map[string]string) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.Variables = vars
	}
}

// WithInitDependencies sets the initDependencies on the Addon.
func WithInitDependencies(deps []addonsv1alpha1.Dependency) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.InitDependencies = deps
	}
}

// createTestAddon creates an Addon with sensible defaults for testing.
//
//nolint:unused // Helper for future tests
func createTestAddon(name string, opts ...AddonOption) *addonsv1alpha1.Addon {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: addonsv1alpha1.AddonSpec{
			Chart:           "test-chart",
			RepoURL:         "https://charts.example.com",
			Version:         "1.0.0",
			TargetCluster:   "in-cluster",
			TargetNamespace: "default",
			Backend: addonsv1alpha1.BackendSpec{
				Type:      "argocd",
				Namespace: "argocd",
			},
		},
	}

	for _, opt := range opts {
		opt(addon)
	}

	ExpectWithOffset(1, k8sClient.Create(ctx, addon)).To(Succeed())

	return addon
}

// createTestAddonValue creates an AddonValue with the given values and labels.
//
//nolint:unparam // Return value used in some tests
func createTestAddonValue(name, addonName string, values map[string]any, extraLabels map[string]string) *addonsv1alpha1.AddonValue {
	labels := map[string]string{
		"addons.in-cloud.io/addon": addonName,
	}
	maps.Copy(labels, extraLabels)

	av := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: addonsv1alpha1.AddonValueSpec{
			Values: string(mustMarshalYAML(values)),
		},
	}

	ExpectWithOffset(1, k8sClient.Create(ctx, av)).To(Succeed())

	return av
}

// mustMarshalYAML marshals v to YAML or panics.
func mustMarshalYAML(v any) []byte {
	data, err := yaml.Marshal(v)
	if err != nil {
		panic(err)
	}

	return data
}

// createTestAddonPhase creates an AddonPhase for testing.
//
//nolint:unused // Helper for future tests
func createTestAddonPhase(name string, rules []addonsv1alpha1.PhaseRule) *addonsv1alpha1.AddonPhase {
	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: rules,
		},
	}

	ExpectWithOffset(1, k8sClient.Create(ctx, phase)).To(Succeed())

	return phase
}

// createTestSecret creates a Secret for testing.
//
//nolint:unparam // namespace parameter kept for flexibility in future tests
func createTestSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}

	ExpectWithOffset(1, k8sClient.Create(ctx, secret)).To(Succeed())

	return secret
}

// createTestConfigMap creates a ConfigMap for testing.
//
//nolint:unused // Helper for future tests
func createTestConfigMap(name, namespace string, data map[string]string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}

	ExpectWithOffset(1, k8sClient.Create(ctx, cm)).To(Succeed())

	return cm
}

// waitForCondition waits for an Addon to have a specific condition status.
//
//nolint:unparam // condType kept generic for reusability across different condition types
func waitForCondition(name string, condType string, status metav1.ConditionStatus) {
	EventuallyWithOffset(1, func() metav1.ConditionStatus {
		addon := &addonsv1alpha1.Addon{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)
		if err != nil {
			return metav1.ConditionUnknown
		}
		for _, c := range addon.Status.Conditions {
			if c.Type == condType {
				return c.Status
			}
		}

		return metav1.ConditionUnknown
	}, timeout, interval).Should(Equal(status))
}

// waitForConditionReason waits for an Addon to have a specific condition reason.
func waitForConditionReason(name string, condType string, reason string) {
	EventuallyWithOffset(1, func() string {
		addon := &addonsv1alpha1.Addon{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)
		if err != nil {
			return ""
		}
		for _, c := range addon.Status.Conditions {
			if c.Type == condType {
				return c.Reason
			}
		}

		return ""
	}, timeout, interval).Should(Equal(reason))
}

// waitForApplication waits for an Application to exist.
//
//nolint:unparam // namespace parameter kept for consistency with other test helpers
func waitForApplication(name, namespace string) *argocdv1alpha1.Application {
	var app *argocdv1alpha1.Application
	EventuallyWithOffset(1, func() error {
		app = &argocdv1alpha1.Application{}

		return k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, app)
	}, timeout, interval).Should(Succeed())

	return app
}

// waitForApplicationNotExist waits for an Application to not exist.
//
//nolint:unused // Helper for future tests
func waitForApplicationNotExist(name, namespace string) {
	EventuallyWithOffset(1, func() bool {
		app := &argocdv1alpha1.Application{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, app)

		return err != nil
	}, timeout, interval).Should(BeTrue())
}

// waitForPhaseRuleMatched waits for an AddonPhase rule to be matched.
func waitForPhaseRuleMatched(name, ruleName string, matched bool) {
	EventuallyWithOffset(1, func() bool {
		phase := &addonsv1alpha1.AddonPhase{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, phase)
		if err != nil {
			return !matched // Return opposite of expected when error
		}
		for _, r := range phase.Status.RuleStatuses {
			if r.Name == ruleName {
				return r.Matched == matched
			}
		}

		return !matched // Rule not found, return opposite
	}, timeout, interval).Should(BeTrue())
}

// waitForAddonPhaseValuesSelector waits for an Addon to have phaseValuesSelector.
func waitForAddonPhaseValuesSelector(name string, hasSelector bool) {
	EventuallyWithOffset(1, func() bool {
		addon := &addonsv1alpha1.Addon{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)
		if err != nil {
			return !hasSelector
		}
		if hasSelector {
			return len(addon.Status.PhaseValuesSelector) > 0
		}

		return len(addon.Status.PhaseValuesSelector) == 0
	}, timeout, interval).Should(BeTrue())
}

// getApplicationValues extracts helm values from an Application.
func getApplicationValues(app *argocdv1alpha1.Application) map[string]any {
	if app.Spec.Source == nil || app.Spec.Source.Helm == nil {
		return nil
	}

	var values map[string]any
	if err := yaml.Unmarshal([]byte(app.Spec.Source.Helm.Values), &values); err != nil {
		return nil
	}

	return values
}

// deleteAddon deletes an Addon if it exists.
//
//nolint:unused // Helper for future tests
func deleteAddon(name string) {
	addon := &addonsv1alpha1.Addon{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)
	if err == nil {
		ExpectWithOffset(1, k8sClient.Delete(ctx, addon)).To(Succeed())
	}
}

// deleteAddonValue deletes an AddonValue if it exists.
//
//nolint:unused // Helper for future tests
func deleteAddonValue(name string) {
	av := &addonsv1alpha1.AddonValue{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, av)
	if err == nil {
		ExpectWithOffset(1, k8sClient.Delete(ctx, av)).To(Succeed())
	}
}

// deleteAddonPhase deletes an AddonPhase if it exists.
//
//nolint:unused // Helper for future tests
func deleteAddonPhase(name string) {
	phase := &addonsv1alpha1.AddonPhase{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, phase)
	if err == nil {
		ExpectWithOffset(1, k8sClient.Delete(ctx, phase)).To(Succeed())
	}
}

// deleteSecret deletes a Secret if it exists.
//
//nolint:unparam // namespace parameter kept for flexibility in future tests
func deleteSecret(name, namespace string) {
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if err == nil {
		ExpectWithOffset(1, k8sClient.Delete(ctx, secret)).To(Succeed())
	}
}
