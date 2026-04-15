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
	"sync"
	"time"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	addonctrl "addons-operator/internal/controller/addon"
	"addons-operator/internal/controller/conditions"
)

var _ = Describe("Addon Controller", func() {
	Context("When reconciling a resource", func() {
		It("should create Application when Addon created", func() {
			name := uniqueName("test-addon")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for Application to be created")
			waitForApplication(name, "argocd")
			By("Verifying ApplicationCreated condition")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("FindAddonsForAddonValue", func() {
		ctx := context.Background()
		It("should return addon when AddonValue labels match selector", func() {
			name := uniqueName("selector-match")
			avName := uniqueName("test-value")
			// Create Addon with a values selector
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
					ValuesSelectors: []addonsv1alpha1.ValuesSelector{{
						Name:        "default",
						Priority:    0,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": name},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			// Create AddonValue with labels matching the selector
			av := &addonsv1alpha1.AddonValue{
				ObjectMeta: metav1.ObjectMeta{
					Name: avName,
					Labels: map[string]string{
						"addons.in-cloud.io/addon": name,
					},
				},
				Spec: addonsv1alpha1.AddonValueSpec{
					Values: "key: value",
				},
			}
			Expect(k8sClient.Create(ctx, av)).To(Succeed())
			// Test the handler function
			controllerReconciler := &addonctrl.AddonReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			requests := controllerReconciler.FindAddonsForAddonValue(ctx, av)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(name))
			Expect(requests[0].Namespace).To(BeEmpty())
			// Cleanup
			Expect(k8sClient.Delete(ctx, av)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
		It("should return multiple addons when AddonValue matches several selectors", func() {
			name1 := uniqueName("multi-match-1")
			name2 := uniqueName("multi-match-2")
			avName := uniqueName("shared-value")
			// Create two Addons with the same selector labels
			sharedLabels := map[string]string{"addons.in-cloud.io/shared": "true"}
			addon1 := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: name1},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-1",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
					ValuesSelectors: []addonsv1alpha1.ValuesSelector{{
						Name:        "shared",
						Priority:    0,
						MatchLabels: sharedLabels,
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon1)).To(Succeed())
			addon2 := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: name2},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-2",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
					ValuesSelectors: []addonsv1alpha1.ValuesSelector{{
						Name:        "shared",
						Priority:    0,
						MatchLabels: sharedLabels,
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon2)).To(Succeed())
			// Create AddonValue matching both selectors
			av := &addonsv1alpha1.AddonValue{
				ObjectMeta: metav1.ObjectMeta{
					Name:   avName,
					Labels: sharedLabels,
				},
				Spec: addonsv1alpha1.AddonValueSpec{
					Values: "shared: true",
				},
			}
			Expect(k8sClient.Create(ctx, av)).To(Succeed())
			// Test: should return both addons
			controllerReconciler := &addonctrl.AddonReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			requests := controllerReconciler.FindAddonsForAddonValue(ctx, av)
			Expect(requests).To(HaveLen(2))
			names := []string{requests[0].Name, requests[1].Name}
			Expect(names).To(ContainElements(name1, name2))
			// Cleanup
			Expect(k8sClient.Delete(ctx, av)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon2)).To(Succeed())
		})
		It("should return empty when no selector matches AddonValue labels", func() {
			name := uniqueName("no-match-addon")
			avName := uniqueName("unmatched-value")
			// Create Addon with specific selector
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "test-chart",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
					ValuesSelectors: []addonsv1alpha1.ValuesSelector{{
						Name:        "specific",
						Priority:    0,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": "other-addon"},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			// Create AddonValue with different labels
			av := &addonsv1alpha1.AddonValue{
				ObjectMeta: metav1.ObjectMeta{
					Name: avName,
					Labels: map[string]string{
						"addons.in-cloud.io/addon": "completely-different",
					},
				},
				Spec: addonsv1alpha1.AddonValueSpec{
					Values: "key: value",
				},
			}
			Expect(k8sClient.Create(ctx, av)).To(Succeed())
			controllerReconciler := &addonctrl.AddonReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			requests := controllerReconciler.FindAddonsForAddonValue(ctx, av)
			// Should not match: addon's selector says "other-addon" but AddonValue has "completely-different"
			Expect(requests).To(BeEmpty())
			// Cleanup
			Expect(k8sClient.Delete(ctx, av)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
		It("should return empty when AddonValue has no addon-prefixed labels", func() {
			avName := uniqueName("no-addon-labels")
			av := &addonsv1alpha1.AddonValue{
				ObjectMeta: metav1.ObjectMeta{
					Name: avName,
					Labels: map[string]string{
						"app.kubernetes.io/name": "test", // Not addon-prefixed
					},
				},
				Spec: addonsv1alpha1.AddonValueSpec{
					Values: "key: value",
				},
			}
			Expect(k8sClient.Create(ctx, av)).To(Succeed())
			controllerReconciler := &addonctrl.AddonReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			requests := controllerReconciler.FindAddonsForAddonValue(ctx, av)
			Expect(requests).To(BeEmpty())
			// Cleanup
			Expect(k8sClient.Delete(ctx, av)).To(Succeed())
		})
	})
	// Note: findDependentAddons functionality is tested via integration tests
	// in the "initDependencies" context below. Direct unit tests were removed
	// because findDependentAddons now uses field indexes which require the
	// manager's cached client (not available in direct unit tests).
	Context("Values Aggregation", func() {
		It("should merge values in priority order", func() {
			addonName := uniqueName("priority-test")
			By("Creating AddonValues with different priorities")
			// Priority 0 (lowest) - will be overwritten
			// Labels must use addons.in-cloud.io/ prefix for exact match
			createTestAddonValue(uniqueName("priority-default"), addonName,
				map[string]any{"a": float64(1), "b": float64(1)},
				map[string]string{"addons.in-cloud.io/layer": "default"},
			)
			// Priority 50 (middle) - overwrites default, overwritten by immutable
			createTestAddonValue(uniqueName("priority-custom"), addonName,
				map[string]any{"b": float64(2), "c": float64(2)},
				map[string]string{"addons.in-cloud.io/layer": "custom"},
			)
			// Priority 99 (highest) - final value
			createTestAddonValue(uniqueName("priority-immutable"), addonName,
				map[string]any{"c": float64(3)},
				map[string]string{"addons.in-cloud.io/layer": "immutable"},
			)
			By("Creating Addon with selectors for each layer")
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonName,
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
					ValuesSelectors: []addonsv1alpha1.ValuesSelector{
						{Name: "default", Priority: 0, MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": addonName, "addons.in-cloud.io/layer": "default"}},
						{Name: "custom", Priority: 50, MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": addonName, "addons.in-cloud.io/layer": "custom"}},
						{Name: "immutable", Priority: 99, MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": addonName, "addons.in-cloud.io/layer": "immutable"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for Application to be created with merged values")
			Eventually(func() map[string]any {
				app := waitForApplication(addonName, "argocd")

				return getApplicationValues(app)
			}, timeout, interval).Should(And(
				HaveKeyWithValue("a", float64(1)), // From default
				HaveKeyWithValue("b", float64(2)), // From custom (overwrites default)
				HaveKeyWithValue("c", float64(3)), // From immutable (overwrites custom)
			))
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("valuesSources", func() {
		// Note: valuesSources extraction and template processing is tested in detail
		// in the sources package. This E2E test verifies the basic integration.
		It("should trigger reconcile when referenced Secret is created", func() {
			addonName := uniqueName("vs-test")
			secretName := uniqueName("test-secret")
			By("Creating Addon with valuesSources referencing non-existent Secret")
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: addonName,
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
					ValuesSources: []addonsv1alpha1.ValueSource{{
						Name: "config",
						SourceRef: addonsv1alpha1.SourceRef{
							APIVersion: "v1",
							Kind:       "Secret",
							Name:       secretName,
							Namespace:  "default",
						},
						Extract: []addonsv1alpha1.ExtractRule{{
							JSONPath: ".data.key",
							As:       "config.key",
							Decode:   "base64",
						}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Verifying Addon has error condition")
			waitForConditionReason(addonName, conditions.TypeValuesResolved, conditions.ReasonValueSourceError)
			By("Creating the missing Secret")
			createTestSecret(secretName, "default", map[string][]byte{
				"key": []byte("testvalue"),
			})
			By("Verifying Addon recovers and creates Application")
			waitForApplication(addonName, "argocd")
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			deleteSecret(secretName, "default")
		})
	})
	Context("Template Error", func() {
		It("should report template error when variable is missing", func() {
			name := uniqueName("template-error")
			By("Creating AddonValue with template referencing missing variable")
			av := &addonsv1alpha1.AddonValue{
				ObjectMeta: metav1.ObjectMeta{
					Name: name + "-values",
					Labels: map[string]string{
						"addons.in-cloud.io/addon": name,
					},
				},
				Spec: addonsv1alpha1.AddonValueSpec{
					Values: "config: {{ .Variables.nonexistent }}",
				},
			}
			Expect(k8sClient.Create(ctx, av)).To(Succeed())
			By("Creating Addon without the variable")
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
					ValuesSelectors: []addonsv1alpha1.ValuesSelector{{
						Name:        "default",
						Priority:    0,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": name},
					}},
					// No Variables defined, so template will fail
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Verifying TemplateError condition is set")
			waitForConditionReason(name, conditions.TypeValuesResolved, conditions.ReasonTemplateError)
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			Expect(k8sClient.Delete(ctx, av)).To(Succeed())
		})
	})

	Context("Infinite Reconcile Protection", func() {
		// This test verifies protection against infinite reconcile cycles.
		// Previously, conditions updating timestamps on every reconcile could cause
		// DDoS-like behavior on the API server.
		//
		// Protection mechanisms:
		// 1. meta.SetStatusCondition preserves LastTransitionTime when status unchanged
		// 2. controller-runtime filters status-only updates via GenerationChangedPredicate
		// 3. ObservedGeneration and ValuesHash are idempotent
		It("should not change condition timestamps when state is stable", func() {
			name := uniqueName("stable-addon")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for Application to be created (initial reconcile complete)")
			waitForApplication(name, "argocd")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
			By("Recording initial condition timestamps")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			initialConditions := make(map[string]metav1.Time)
			for _, c := range addon.Status.Conditions {
				initialConditions[c.Type] = c.LastTransitionTime
			}
			initialValuesHash := addon.Status.ValuesHash
			initialObservedGen := addon.Status.ObservedGeneration
			Expect(initialConditions).NotTo(BeEmpty(), "Should have conditions after initial reconcile")
			By("Waiting to ensure no spontaneous updates")
			// Wait longer than typical reconcile interval to catch any spurious updates
			Consistently(func() bool {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return false
				}
				// Check all condition timestamps remain unchanged
				for _, c := range current.Status.Conditions {
					initial, exists := initialConditions[c.Type]
					if !exists {
						// New condition appeared - not stable
						return false
					}
					if !c.LastTransitionTime.Equal(&initial) {
						// Timestamp changed - infinite loop detected!
						return false
					}
				}
				// Verify idempotent fields
				if current.Status.ValuesHash != initialValuesHash {
					return false
				}
				if current.Status.ObservedGeneration != initialObservedGen {
					return false
				}

				return true
			}, 3*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Conditions and idempotent fields should remain stable over time")
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
		It("should preserve LastTransitionTime when condition status unchanged after manual reconcile trigger", func() {
			name := uniqueName("manual-trigger")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for stable state")
			waitForApplication(name, "argocd")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
			By("Recording timestamps before trigger")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			timestampsBefore := make(map[string]metav1.Time)
			for _, c := range addon.Status.Conditions {
				timestampsBefore[c.Type] = c.LastTransitionTime
			}
			By("Triggering reconcile via annotation update")
			// This triggers a reconcile because spec changes increment generation
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			if addon.Annotations == nil {
				addon.Annotations = make(map[string]string)
			}
			addon.Annotations["test.trigger"] = "reconcile"
			Expect(k8sClient.Update(ctx, addon)).To(Succeed())
			By("Waiting for reconcile to process")
			// Give time for reconcile to run
			Eventually(func() string {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return ""
				}

				return current.Annotations["test.trigger"]
			}, timeout, interval).Should(Equal("reconcile"))
			By("Verifying timestamps remain unchanged over time")
			// Use Consistently to verify no updates occur within the check period
			Consistently(func() bool {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return false
				}
				for _, c := range current.Status.Conditions {
					before, exists := timestampsBefore[c.Type]
					if !exists || !c.LastTransitionTime.Equal(&before) {
						return false
					}
				}

				return true
			}, 1*time.Second, 200*time.Millisecond).Should(BeTrue(),
				"LastTransitionTime should not change when status is unchanged")
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("Cascade Delete", func() {
		// Note: In envtest, garbage collection (ownerReference cascade delete)
		// doesn't work automatically because GC is a separate controller.
		// This test verifies ownerReference is set correctly; actual GC works in real clusters.
		It("should set ownerReference on Application", func() {
			name := uniqueName("cascade-test")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for Application to be created")
			app := waitForApplication(name, "argocd")
			By("Verifying ownerReference on Application")
			Expect(app.OwnerReferences).To(HaveLen(1))
			Expect(app.OwnerReferences[0].Kind).To(Equal("Addon"))
			Expect(app.OwnerReferences[0].Name).To(Equal(name))
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("Negative Cases", func() {
		It("should handle non-existent ArgoCD namespace gracefully", func() {
			name := uniqueName("bad-namespace")
			By("Creating Addon with non-existent backend namespace")
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
						Namespace: "nonexistent-argocd-namespace-xyz",
					},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Verifying ApplicationCreated condition shows error")
			waitForConditionReason(name, conditions.TypeApplicationCreated, conditions.ReasonApplicationError)
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("Circular Dependencies", func() {
		// Helper to create a dependency with required criteria (waiting for Ready condition)
		readyCriteria := []addonsv1alpha1.Criterion{
			{
				JSONPath: "$.status.conditions[?@.type=='Ready'].status",
				Operator: addonsv1alpha1.OperatorEqual,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"True"`)},
			},
		}
		It("should detect self-dependency and wait indefinitely", func() {
			name := uniqueName("self-dep")
			By("Creating Addon that depends on itself")
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
					InitDependencies: []addonsv1alpha1.Dependency{
						{Name: name, Criteria: readyCriteria}, // Self-reference
					},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Verifying WaitingForDependency condition is set")
			waitForConditionReason(name, conditions.TypeDependenciesMet, conditions.ReasonWaitingForDependency)
			By("Verifying no Application is created")
			Consistently(func() error {
				app := &argocdv1alpha1.Application{}

				return k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app)
			}, 2*time.Second, 200*time.Millisecond).ShouldNot(Succeed())
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
		It("should handle simple circular dependency (A depends on B, B depends on A)", func() {
			nameA := uniqueName("circ-a")
			nameB := uniqueName("circ-b")
			By("Creating Addon A that depends on B")
			addonA := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: nameA,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-a",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
					},
					InitDependencies: []addonsv1alpha1.Dependency{
						{Name: nameB, Criteria: readyCriteria},
					},
				},
			}
			Expect(k8sClient.Create(ctx, addonA)).To(Succeed())
			By("Creating Addon B that depends on A")
			addonB := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: nameB,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-b",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
					},
					InitDependencies: []addonsv1alpha1.Dependency{
						{Name: nameA, Criteria: readyCriteria},
					},
				},
			}
			Expect(k8sClient.Create(ctx, addonB)).To(Succeed())
			By("Verifying both Addons are waiting for dependencies")
			waitForConditionReason(nameA, conditions.TypeDependenciesMet, conditions.ReasonWaitingForDependency)
			waitForConditionReason(nameB, conditions.TypeDependenciesMet, conditions.ReasonWaitingForDependency)
			By("Verifying no Applications are created")
			Consistently(func() bool {
				appA := &argocdv1alpha1.Application{}
				errA := k8sClient.Get(ctx, types.NamespacedName{Name: nameA, Namespace: "argocd"}, appA)
				appB := &argocdv1alpha1.Application{}
				errB := k8sClient.Get(ctx, types.NamespacedName{Name: nameB, Namespace: "argocd"}, appB)

				return errA != nil && errB != nil // Both should not exist
			}, 2*time.Second, 200*time.Millisecond).Should(BeTrue())
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addonA)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addonB)).To(Succeed())
		})
	})
	Context("initDependencies", func() {
		// Note: Testing dynamic dependency resolution (status changes triggering unblock)
		// is complex in envtest due to timing. This test verifies the blocking behavior.
		It("should set WaitingForDependencies when dependency not ready", func() {
			depName := uniqueName("dep-addon")
			blockedName := uniqueName("blocked-addon")
			By("Creating dependency Addon (not ready)")
			depAddon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: depName,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "dep-chart",
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
			Expect(k8sClient.Create(ctx, depAddon)).To(Succeed())
			By("Creating blocked Addon with initDependencies")
			blockedAddon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: blockedName,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "blocked-chart",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
					},
					InitDependencies: []addonsv1alpha1.Dependency{
						{
							Name: depName,
							Criteria: []addonsv1alpha1.Criterion{
								{
									JSONPath: "$.status.conditions[0].status",
									Operator: addonsv1alpha1.OperatorEqual,
									Value:    &apiextensionsv1.JSON{Raw: []byte(`"True"`)},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, blockedAddon)).To(Succeed())
			By("Verifying WaitingForDependency condition is set")
			waitForConditionReason(blockedName, conditions.TypeDependenciesMet, conditions.ReasonWaitingForDependency)
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, blockedAddon)).To(Succeed())
			Expect(k8sClient.Delete(ctx, depAddon)).To(Succeed())
		})
		It("should unblock Application when dependency becomes Ready", func() {
			depName := uniqueName("dep-unblock")
			blockedName := uniqueName("blocked-unblock")
			By("Creating dependency Addon")
			depAddon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: depName,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "dep-chart",
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
			Expect(k8sClient.Create(ctx, depAddon)).To(Succeed())
			By("Creating blocked Addon with initDependencies checking DependenciesMet condition")
			// Use filter-based JSONPath to check specific condition by type
			// This is more stable than checking conditions[0] which depends on order
			blockedAddon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: blockedName,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "blocked-chart",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
					},
					InitDependencies: []addonsv1alpha1.Dependency{
						{
							Name: depName,
							Criteria: []addonsv1alpha1.Criterion{
								{
									// Check that DependenciesMet condition is True
									// This condition is stable and set early in reconciliation
									JSONPath: "$.status.conditions[?@.type=='DependenciesMet'].status",
									Operator: addonsv1alpha1.OperatorEqual,
									Value:    &apiextensionsv1.JSON{Raw: []byte(`"True"`)},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, blockedAddon)).To(Succeed())
			By("Verifying dependency Addon has DependenciesMet=True")
			// Dependency addon has no dependencies itself, so DependenciesMet becomes True immediately
			Eventually(func() bool {
				addon := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: depName}, addon); err != nil {
					return false
				}
				cond := meta.FindStatusCondition(addon.Status.Conditions, conditions.TypeDependenciesMet)

				return cond != nil && cond.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())
			By("Verifying blocked Addon Application is created since dependency is ready")
			// Since dependency has DependenciesMet=True immediately, the blocked addon
			// should create its Application without needing to wait
			Eventually(func() error {
				app := &argocdv1alpha1.Application{}

				return k8sClient.Get(ctx, types.NamespacedName{Name: blockedName, Namespace: "argocd"}, app)
			}, 60*time.Second, interval).Should(Succeed())
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, blockedAddon)).To(Succeed())
			Expect(k8sClient.Delete(ctx, depAddon)).To(Succeed())
		})
	})
	Context("Concurrent Reconciliation", func() {
		// Tests for thread-safety during concurrent updates.
		// Verifies the controller handles race conditions gracefully.
		It("should handle concurrent annotation updates safely", func() {
			name := uniqueName("concurrent-test")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for initial reconciliation")
			waitForApplication(name, "argocd")
			By("Triggering concurrent annotation updates")
			var wg sync.WaitGroup
			errChan := make(chan error, 10)
			for i := range 10 {
				wg.Add(1)
				go func(n int) {
					defer wg.Done()
					// Each goroutine fetches fresh version and updates
					for range 3 {
						current := &addonsv1alpha1.Addon{}
						if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
							continue
						}
						if current.Annotations == nil {
							current.Annotations = make(map[string]string)
						}
						current.Annotations[fmt.Sprintf("test.concurrent.%d", n)] = fmt.Sprintf("value-%d", n)
						if err := k8sClient.Update(ctx, current); err == nil {
							return // Success
						}
						// Retry on conflict
					}
					errChan <- fmt.Errorf("goroutine %d failed after retries", n)
				}(i)
			}
			wg.Wait()
			close(errChan)
			// Check if any goroutine had permanent failure
			var errors []error
			for err := range errChan {
				errors = append(errors, err)
			}
			// Some failures are OK due to conflicts, but not all should fail
			Expect(len(errors)).To(BeNumerically("<", 10), "Most concurrent updates should succeed")
			By("Verifying Addon is still in consistent state")
			Eventually(func() bool {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return false
				}
				// Check at least some annotations were added
				return len(current.Annotations) > 0
			}, timeout, interval).Should(BeTrue())
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
		It("should handle concurrent spec updates with version changes", func() {
			name := uniqueName("concurrent-version")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for initial reconciliation")
			waitForApplication(name, "argocd")
			By("Triggering sequential version updates")
			// Sequential updates to avoid too many conflicts
			for i := 1; i <= 5; i++ {
				Eventually(func() error {
					current := &addonsv1alpha1.Addon{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
						return err
					}
					current.Spec.Version = fmt.Sprintf("1.0.%d", i)

					return k8sClient.Update(ctx, current)
				}, timeout, interval).Should(Succeed())
			}
			By("Verifying final state is consistent")
			Eventually(func() string {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return ""
				}

				return current.Spec.Version
			}, timeout, interval).Should(Equal("1.0.5"))
			By("Verifying Application reflects latest version")
			Eventually(func() string {
				app := &argocdv1alpha1.Application{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app); err != nil {
					return ""
				}

				return app.Spec.Source.TargetRevision
			}, timeout, interval).Should(Equal("1.0.5"))
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("Pause Annotation", func() {
		It("should skip reconciliation when paused annotation is set", func() {
			name := uniqueName("paused-addon")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for initial Application creation")
			waitForApplication(name, "argocd")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
			By("Recording initial Application state")
			initialApp := &argocdv1alpha1.Application{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, initialApp)).To(Succeed())
			initialResourceVersion := initialApp.ResourceVersion
			By("Adding pause annotation")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			if addon.Annotations == nil {
				addon.Annotations = make(map[string]string)
			}
			addon.Annotations["addons.in-cloud.io/paused"] = "true"
			Expect(k8sClient.Update(ctx, addon)).To(Succeed())
			By("Verifying Ready condition shows Paused reason")
			Eventually(func() string {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return ""
				}
				cond := meta.FindStatusCondition(current.Status.Conditions, conditions.TypeReady)
				if cond == nil {
					return ""
				}

				return cond.Reason
			}, timeout, interval).Should(Equal(conditions.ReasonPaused))
			By("Verifying Progressing is False when paused")
			Eventually(func() metav1.ConditionStatus {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return ""
				}
				cond := meta.FindStatusCondition(current.Status.Conditions, conditions.TypeProgressing)
				if cond == nil {
					return ""
				}

				return cond.Status
			}, timeout, interval).Should(Equal(metav1.ConditionFalse))
			By("Updating Addon spec while paused")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			addon.Spec.Version = "2.0.0"
			Expect(k8sClient.Update(ctx, addon)).To(Succeed())
			By("Verifying Application is not updated while paused")
			Consistently(func() string {
				app := &argocdv1alpha1.Application{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app); err != nil {
					return ""
				}

				return app.Spec.Source.TargetRevision
			}, 2*time.Second, 200*time.Millisecond).Should(Equal("1.0.0"))
			// Also verify resource version didn't change (no updates happened)
			currentApp := &argocdv1alpha1.Application{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, currentApp)).To(Succeed())
			Expect(currentApp.ResourceVersion).To(Equal(initialResourceVersion))
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
		It("should resume reconciliation when pause annotation is removed", func() {
			name := uniqueName("resume-addon")
			By("Creating paused Addon")
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
					Annotations: map[string]string{
						"addons.in-cloud.io/paused": "true",
					},
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Verifying Addon is paused (Ready=False, Reason=Paused)")
			Eventually(func() string {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return ""
				}
				cond := meta.FindStatusCondition(current.Status.Conditions, conditions.TypeReady)
				if cond == nil {
					return ""
				}

				return cond.Reason
			}, timeout, interval).Should(Equal(conditions.ReasonPaused))
			By("Verifying no Application is created while paused")
			Consistently(func() error {
				app := &argocdv1alpha1.Application{}

				return k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app)
			}, 2*time.Second, 200*time.Millisecond).ShouldNot(Succeed())
			By("Removing pause annotation")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			delete(addon.Annotations, "addons.in-cloud.io/paused")
			Expect(k8sClient.Update(ctx, addon)).To(Succeed())
			By("Verifying Application is created after unpause")
			waitForApplication(name, "argocd")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
		It("should allow deletion even when paused", func() {
			name := uniqueName("paused-delete")
			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			By("Waiting for Application to be created")
			waitForApplication(name, "argocd")
			By("Adding pause annotation")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			if addon.Annotations == nil {
				addon.Annotations = make(map[string]string)
			}
			addon.Annotations["addons.in-cloud.io/paused"] = "true"
			Expect(k8sClient.Update(ctx, addon)).To(Succeed())
			By("Verifying Addon is paused (Ready=False, Reason=Paused)")
			Eventually(func() string {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return ""
				}
				cond := meta.FindStatusCondition(current.Status.Conditions, conditions.TypeReady)
				if cond == nil {
					return ""
				}

				return cond.Reason
			}, timeout, interval).Should(Equal(conditions.ReasonPaused))
			By("Deleting the paused Addon")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			By("Verifying Addon is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &addonsv1alpha1.Addon{})

				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
			By("Verifying Application is also deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, &argocdv1alpha1.Application{})

				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
	Context("Values Stabilization", func() {
		// These tests verify the stabilization gate that prevents creating Application
		// with incomplete values due to race conditions (informer cache lag, AddonPhase not ready).
		It("should set ValuesHash before creating Application (stabilization gate)", func() {
			name := uniqueName("stabilize-first")

			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Verifying Application is eventually created after stabilization")
			waitForApplication(name, "argocd")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)

			By("Verifying ValuesHash is set")
			Eventually(func() bool {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return false
				}

				return current.Status.ValuesHash != ""
			}, timeout, interval).Should(BeTrue())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})

		It("should create Application after hash stabilizes", func() {
			name := uniqueName("stabilize-complete")

			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Waiting for Application to be created (stabilization complete)")
			waitForApplication(name, "argocd")

			By("Verifying ValuesHash is set")
			Eventually(func() bool {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return false
				}

				return current.Status.ValuesHash != ""
			}, timeout, interval).Should(BeTrue())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})

		It("should defer Application creation if values change between reconciles", func() {
			name := uniqueName("stabilize-late-value")
			avName := uniqueName("late-value")

			By("Creating the Addon with valuesSelector")
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
					ValuesSelectors: []addonsv1alpha1.ValuesSelector{{
						Name:        "default",
						Priority:    0,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": name},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Creating AddonValue shortly after Addon (simulating late arrival)")
			// This simulates the race condition where AddonValue arrives after first reconcile
			av := &addonsv1alpha1.AddonValue{
				ObjectMeta: metav1.ObjectMeta{
					Name: avName,
					Labels: map[string]string{
						"addons.in-cloud.io/addon": name,
					},
				},
				Spec: addonsv1alpha1.AddonValueSpec{
					Values: "lateKey: lateValue",
				},
			}
			Expect(k8sClient.Create(ctx, av)).To(Succeed())

			By("Waiting for Application to be created with the values")
			Eventually(func() map[string]any {
				app := &argocdv1alpha1.Application{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app); err != nil {
					return nil
				}

				return getApplicationValues(app)
			}, timeout, interval).Should(HaveKeyWithValue("lateKey", "lateValue"))

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, av)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})

		It("should update existing Application without stabilization delay", func() {
			name := uniqueName("stabilize-update")

			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Waiting for Application to be created")
			waitForApplication(name, "argocd")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)

			By("Recording initial state")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			initialHash := addon.Status.ValuesHash

			By("Updating Addon version (triggers Application update)")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			addon.Spec.Version = "2.0.0"
			Expect(k8sClient.Update(ctx, addon)).To(Succeed())

			By("Verifying Application is updated without WaitingForStableValues")
			// Updates to existing Application should bypass the stabilization gate
			Eventually(func() string {
				app := &argocdv1alpha1.Application{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app); err != nil {
					return ""
				}

				return app.Spec.Source.TargetRevision
			}, timeout, interval).Should(Equal("2.0.0"))

			By("Verifying hash is updated (spec change doesn't require stabilization)")
			// For existing Application, hash just changes - no stabilization needed
			Eventually(func() string {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return ""
				}

				return current.Status.ValuesHash
			}, timeout, interval).Should(Equal(initialHash)) // Hash is from values only, version doesn't change it

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("Deployed Latching", func() {
		It("should set Deployed=true when Synced+Healthy and keep it true when unhealthy", func() {
			name := uniqueName("deployed-latch")

			By("Creating the Addon")
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Waiting for Application to be created")
			waitForApplication(name, "argocd")

			By("Verifying Deployed is initially false")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			Expect(addon.Status.Deployed).To(BeFalse())

			By("Setting Application status to Synced+Healthy")
			Eventually(func() error {
				app := &argocdv1alpha1.Application{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app); err != nil {
					return err
				}
				app.Status.Sync.Status = argocdv1alpha1.SyncStatusCodeSynced
				if app.Spec.Source != nil {
					app.Status.Sync.ComparedTo.Source = *app.Spec.Source
				}
				app.Status.Health.Status = health.HealthStatusHealthy

				return k8sClient.Update(ctx, app)
			}, timeout, interval).Should(Succeed())

			By("Verifying Deployed becomes true")
			Eventually(func() bool {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return false
				}

				return current.Status.Deployed
			}, timeout, interval).Should(BeTrue())

			By("Verifying Deployed condition is set with correct timestamp")
			waitForCondition(name, conditions.TypeDeployed, metav1.ConditionTrue)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			deployedCond := meta.FindStatusCondition(addon.Status.Conditions, conditions.TypeDeployed)
			Expect(deployedCond).NotTo(BeNil())
			Expect(deployedCond.Reason).To(Equal("Deployed"))
			deployedTime := deployedCond.LastTransitionTime

			By("Setting Application status to Degraded")
			Eventually(func() error {
				app := &argocdv1alpha1.Application{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app); err != nil {
					return err
				}
				app.Status.Health.Status = health.HealthStatusDegraded

				return k8sClient.Update(ctx, app)
			}, timeout, interval).Should(Succeed())

			By("Verifying Deployed stays true even when unhealthy")
			// Wait for the controller to process the degraded status
			waitForConditionReason(name, conditions.TypeHealthy, "Degraded")
			// After processing, Deployed must still be true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)).To(Succeed())
			Expect(addon.Status.Deployed).To(BeTrue())

			By("Verifying Deployed condition timestamp is preserved after status change")
			currentDeployedCond := meta.FindStatusCondition(addon.Status.Conditions, conditions.TypeDeployed)
			Expect(currentDeployedCond).NotTo(BeNil())
			Expect(currentDeployedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(currentDeployedCond.LastTransitionTime).To(Equal(deployedTime))

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})

		// Regression for the scenario Dmitry reproduced by freezing Argo pods:
		// an Application can carry Sync=Synced + Health=Healthy from a previous
		// deploy, while its ComparedTo still points at the old spec. In that
		// state Deployed must stay false — otherwise the operator latches
		// Deployed=true against a spec Argo has never observed.
		It("should NOT set Deployed=true when Sync/Health are stale vs spec.source", func() {
			name := uniqueName("addon-stale")

			By("Creating the Addon")
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: name},
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Waiting for Application to be created")
			waitForApplication(name, "argocd")

			By("Simulating stale Argo status: Sync=Synced + Health=Healthy but ComparedTo points to a different chart version")
			Eventually(func() error {
				app := &argocdv1alpha1.Application{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "argocd"}, app); err != nil {
					return err
				}
				app.Status.Sync.Status = argocdv1alpha1.SyncStatusCodeSynced
				app.Status.Health.Status = health.HealthStatusHealthy
				// ComparedTo reflects an older spec that the operator has since overwritten.
				if app.Spec.Source != nil {
					stale := *app.Spec.Source
					stale.TargetRevision = "0.0.1-stale"
					app.Status.Sync.ComparedTo.Source = stale
				}

				return k8sClient.Update(ctx, app)
			}, timeout, interval).Should(Succeed())

			By("Verifying Deployed stays false for long enough that the controller has reconciled")
			Consistently(func() bool {
				current := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, current); err != nil {
					return true
				}

				return current.Status.Deployed
			}, 3*time.Second, 500*time.Millisecond).Should(BeFalse())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
	Context("No-op reconciliation", func() {
		It("should not update ResourceVersion after stabilization", func() {
			name := uniqueName("addon-noop")

			By("Creating the Addon")
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: name},
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
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Waiting for Application to be created and conditions to stabilize")
			waitForApplication(name, "argocd")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)

			By("Capturing ResourceVersion after stabilization")
			stabilized := &addonsv1alpha1.Addon{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, stabilized)).To(Succeed())
			rv := stabilized.ResourceVersion

			By("Verifying ResourceVersion stays the same for 3 seconds")
			Consistently(func() string {
				a := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, a); err != nil {
					return ""
				}

				return a.ResourceVersion
			}, 3*time.Second, 500*time.Millisecond).Should(Equal(rv))

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})

	Context("Large Scale Scenarios", Label("large-scale"), func() {
		// These tests verify behavior with multiple Addons.
		// Use "ginkgo --label-filter=large-scale" to run these specifically.
		It("should handle 20 addons efficiently", func() {
			const numAddons = 20
			addons := make([]*addonsv1alpha1.Addon, numAddons)
			By(fmt.Sprintf("Creating %d Addons", numAddons))
			for i := range numAddons {
				addon := &addonsv1alpha1.Addon{
					ObjectMeta: metav1.ObjectMeta{
						Name: uniqueName(fmt.Sprintf("scale-%03d", i)),
					},
					Spec: addonsv1alpha1.AddonSpec{
						Chart:           fmt.Sprintf("chart-%03d", i),
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
				addons[i] = addon
				Expect(k8sClient.Create(ctx, addon)).To(Succeed())
			}
			By("Verifying all Applications are created")
			Eventually(func() int {
				var appList argocdv1alpha1.ApplicationList
				if err := k8sClient.List(ctx, &appList); err != nil {
					return 0
				}
				// Count apps that match our test prefix
				count := 0
				for _, app := range appList.Items {
					for _, a := range addons {
						if app.Name == a.Name {
							count++

							break
						}
					}
				}

				return count
			}, 2*time.Minute, time.Second).Should(Equal(numAddons))
			By("Verifying all Addons have ApplicationCreated condition")
			for _, addon := range addons {
				waitForCondition(addon.Name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
			}
			By("Cleanup")
			for _, addon := range addons {
				Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			}
		})
	})
})
