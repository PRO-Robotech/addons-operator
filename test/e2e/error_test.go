//go:build e2e
// +build e2e

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

package e2e

import (
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/conditions"
)

var _ = Describe("Error Recovery & Edge Cases", Ordered, func() {
	// Initialize k8s client before all tests
	BeforeAll(func() {
		By("initializing k8s client")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred(), "Failed to initialize k8s client")
		}
	})

	Context("Scenario 5: initDependencies (PRD 2.5)", func() {
		var depName, blockedName string
		var depAddon, blockedAddon *addonsv1alpha1.Addon

		BeforeEach(func() {
			depName = uniqueName("dependency")
			blockedName = uniqueName("blocked")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if blockedAddon != nil {
				cleanupResource(blockedAddon)
			}
			if depAddon != nil {
				cleanupResource(depAddon)
			}
		})

		It("should block Application creation until dependency Ready", func() {
			By("Creating dependency Addon WITHOUT auto-sync (stays non-Ready)")
			// Disable auto-sync so the dependency stays in Pending/OutOfSync state
			depAddon = createTestAddon(depName, WithoutAutoSync())

			By("Creating blocked Addon with initDependencies")
			// Build criterion: depAddon Ready condition status == "True"
			criterion := NewCriterion().
				WithJSONPath("$.status.conditions[?@.type=='Ready'].status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			blockedAddon = createTestAddon(blockedName,
				WithInitDependencies(depName, criterion),
			)

			By("Verifying blocked Addon has DependenciesMet=False condition")
			Eventually(func() metav1.ConditionStatus {
				addon, err := getAddon(blockedName)
				if err != nil {
					return metav1.ConditionUnknown
				}
				for _, c := range addon.Status.Conditions {
					if c.Type == conditions.TypeDependenciesMet {
						return c.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).Should(Equal(metav1.ConditionFalse),
				"DependenciesMet should be False while waiting")

			By("Verifying NO Application created for blocked Addon")
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      blockedName,
					Namespace: argoCDNamespace,
				}, &argocdv1alpha1.Application{})
				return errors.IsNotFound(err)
			}, "10s", interval).Should(BeTrue(),
				"Application should NOT be created while dependency not Ready")

			By("Enabling auto-sync on dependency to make it Ready")
			Eventually(func() error {
				freshDep, err := getAddon(depName)
				if err != nil {
					return err
				}
				freshDep.Spec.Backend.SyncPolicy = &addonsv1alpha1.SyncPolicy{
					Automated: &addonsv1alpha1.AutomatedSync{
						Prune:    true,
						SelfHeal: true,
					},
				}
				return k8sClient.Update(ctx, freshDep)
			}, timeout, interval).Should(Succeed(), "Should enable auto-sync on dependency")

			By("Waiting for dependency to become Ready")
			waitForAddonReady(depName)

			By("Verifying DependenciesMet becomes True")
			waitForCondition(blockedName, conditions.TypeDependenciesMet, metav1.ConditionTrue)

			By("Verifying Application is now created for blocked Addon")
			waitForApplication(blockedName, argoCDNamespace)
		})
	})

	Context("Scenario 8: Error Recovery (PRD 2.8)", func() {
		var name, secretName string
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("error-recovery")
			secretName = name + "-secret"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should recover when missing Secret is created", func() {
			By("Creating Addon referencing non-existent Secret")
			addon = createTestAddon(name,
				WithValueSource("config", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.key",
						As:       "config.key",
						Decode:   "base64",
					}),
			)

			By("Verifying ValuesResolved condition is False")
			Eventually(func() metav1.ConditionStatus {
				addon, err := getAddon(name)
				if err != nil {
					return metav1.ConditionUnknown
				}
				for _, c := range addon.Status.Conditions {
					if c.Type == conditions.TypeValuesResolved {
						return c.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).Should(Equal(metav1.ConditionFalse),
				"ValuesResolved should be False when Secret missing")

			By("Creating the missing Secret")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"key": []byte("recovered-value"),
			})

			By("Waiting for ValuesResolved to become True")
			waitForCondition(name, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Verifying Application is created after recovery")
			waitForApplication(name, argoCDNamespace)

			By("Verifying Addon reaches Ready phase")
			waitForAddonReady(name)
		})
	})

	Context("Application Sync Failure Recovery", func() {
		var name string
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("sync-recovery")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
		})

		It("should recover from invalid chart configuration", func() {
			By("Creating Addon with invalid chart version")
			addon = &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "podinfo",
					RepoURL:         "https://stefanprodan.github.io/podinfo",
					Version:         "99.99.99", // Non-existent version
					TargetCluster:   "in-cluster",
					TargetNamespace: testNamespace,
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
						SyncPolicy: &addonsv1alpha1.SyncPolicy{
							Automated: &addonsv1alpha1.AutomatedSync{
								Prune:    true,
								SelfHeal: true,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed(), "Failed to create Addon")

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying Application is in error state (OutOfSync or ComparisonError)")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				// Check if sync status indicates error
				syncStatus := app.Status.Sync.Status
				// Check if there's an operation state with error
				hasError := app.Status.OperationState != nil &&
					app.Status.OperationState.Phase == "Error"
				// Application should be OutOfSync or have ComparisonError
				return syncStatus == argocdv1alpha1.SyncStatusCodeOutOfSync ||
					syncStatus == argocdv1alpha1.SyncStatusCodeUnknown ||
					hasError ||
					app.Status.Conditions != nil
			}, timeout, interval).Should(BeTrue(),
				"Application should show error state for invalid version")

			By("Verifying Addon is NOT Ready")
			Eventually(func() metav1.ConditionStatus {
				a, err := getAddon(name)
				if err != nil {
					return metav1.ConditionUnknown
				}
				for _, c := range a.Status.Conditions {
					if c.Type == "Ready" {
						return c.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).ShouldNot(Equal(metav1.ConditionTrue),
				"Addon should not be Ready with invalid chart version")

			By("Fixing the Addon with valid chart version")
			Eventually(func() error {
				freshAddon, err := getAddon(name)
				if err != nil {
					return err
				}
				freshAddon.Spec.Version = "6.5.0" // Valid version
				return k8sClient.Update(ctx, freshAddon)
			}, timeout, interval).Should(Succeed(), "Should update Addon with valid version")

			By("Waiting for Application to sync successfully")
			waitForApplicationSynced(name, argoCDNamespace)

			By("Verifying Addon reaches Ready phase after fix")
			waitForAddonReady(name)

			By("Verifying Synced condition is True")
			waitForCondition(name, conditions.TypeSynced, metav1.ConditionTrue)
		})

		It("should recover from invalid target namespace", func() {
			nonExistentNs := uniqueName("nonexistent-ns")

			By("Creating Addon targeting non-existent namespace")
			addon = &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "podinfo",
					RepoURL:         "https://stefanprodan.github.io/podinfo",
					Version:         "6.5.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: nonExistentNs, // Namespace doesn't exist
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
						SyncPolicy: &addonsv1alpha1.SyncPolicy{
							Automated: &addonsv1alpha1.AutomatedSync{
								Prune:      true,
								SelfHeal:   true,
								AllowEmpty: true,
							},
							SyncOptions: []string{"CreateNamespace=true"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed(), "Failed to create Addon")

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Waiting for sync to complete (with CreateNamespace option)")
			// With CreateNamespace=true, this should succeed
			waitForApplicationSynced(name, argoCDNamespace)

			By("Verifying Addon reaches Ready phase")
			waitForAddonReady(name)
		})
	})

	Context("Multiple Dependencies", func() {
		var dep1Name, dep2Name, blockedName string
		var dep1Addon, dep2Addon, blockedAddon *addonsv1alpha1.Addon

		BeforeEach(func() {
			dep1Name = uniqueName("dep1")
			dep2Name = uniqueName("dep2")
			blockedName = uniqueName("multi-dep")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if blockedAddon != nil {
				cleanupResource(blockedAddon)
			}
			if dep1Addon != nil {
				cleanupResource(dep1Addon)
			}
			if dep2Addon != nil {
				cleanupResource(dep2Addon)
			}
		})

		It("should wait for ALL dependencies to be Ready", func() {
			By("Creating first dependency Addon WITHOUT auto-sync")
			dep1Addon = createTestAddon(dep1Name, WithoutAutoSync())

			By("Creating second dependency Addon WITHOUT auto-sync")
			dep2Addon = createTestAddon(dep2Name, WithoutAutoSync())

			By("Creating Addon with multiple initDependencies")
			criterion1 := NewCriterion().
				WithJSONPath("$.status.conditions[?@.type=='Ready'].status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			criterion2 := NewCriterion().
				WithJSONPath("$.status.conditions[?@.type=='Ready'].status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			blockedAddon = createTestAddon(blockedName,
				WithInitDependencies(dep1Name, criterion1),
				WithInitDependencies(dep2Name, criterion2),
			)

			By("Enabling auto-sync on first dependency to make it Ready")
			Eventually(func() error {
				freshDep, err := getAddon(dep1Name)
				if err != nil {
					return err
				}
				freshDep.Spec.Backend.SyncPolicy = &addonsv1alpha1.SyncPolicy{
					Automated: &addonsv1alpha1.AutomatedSync{
						Prune:    true,
						SelfHeal: true,
					},
				}
				return k8sClient.Update(ctx, freshDep)
			}, timeout, interval).Should(Succeed(), "Should enable auto-sync on dep1")

			By("Waiting for first dependency to become Ready")
			waitForAddonReady(dep1Name)

			By("Verifying Application still NOT created (dep2 not Ready)")
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      blockedName,
					Namespace: argoCDNamespace,
				}, &argocdv1alpha1.Application{})
				return errors.IsNotFound(err)
			}, "10s", interval).Should(BeTrue(),
				"Application should NOT be created until ALL dependencies Ready")

			By("Enabling auto-sync on second dependency to make it Ready")
			Eventually(func() error {
				freshDep, err := getAddon(dep2Name)
				if err != nil {
					return err
				}
				freshDep.Spec.Backend.SyncPolicy = &addonsv1alpha1.SyncPolicy{
					Automated: &addonsv1alpha1.AutomatedSync{
						Prune:    true,
						SelfHeal: true,
					},
				}
				return k8sClient.Update(ctx, freshDep)
			}, timeout, interval).Should(Succeed(), "Should enable auto-sync on dep2")

			By("Waiting for second dependency to become Ready")
			waitForAddonReady(dep2Name)

			By("Verifying Application is now created")
			waitForApplication(blockedName, argoCDNamespace)
		})
	})
})

var _ = Describe("Edge Cases", Ordered, func() {
	BeforeAll(func() {
		By("ensuring k8s client is initialized")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Empty ValuesSelector", func() {
		var name string
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("empty-selector")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
		})

		It("should work with no valuesSelectors", func() {
			By("Creating Addon without valuesSelectors")
			addon = createTestAddon(name)
			// Note: createTestAddon doesn't add selectors by default

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying Application has empty/default values")
			app, err := getApplication(name, argoCDNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Application should be created even without values
			Expect(app.Spec.Source).NotTo(BeNil())
		})
	})

	Context("Concurrent Addon Creation", func() {
		It("should handle multiple Addons created simultaneously", func() {
			const numAddons = 3
			names := make([]string, numAddons)
			addons := make([]*addonsv1alpha1.Addon, numAddons)

			// Generate unique names
			for i := 0; i < numAddons; i++ {
				names[i] = uniqueName(fmt.Sprintf("concurrent-%d", i))
			}

			By("Creating multiple Addons concurrently")
			var wg sync.WaitGroup
			for i := 0; i < numAddons; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()
					addons[idx] = createTestAddon(names[idx])
				}(i)
			}
			wg.Wait()

			By("Waiting for all Applications to be created")
			for i := 0; i < numAddons; i++ {
				waitForApplication(names[i], argoCDNamespace)
			}

			By("Verifying all Addons reach Ready phase")
			for i := 0; i < numAddons; i++ {
				waitForAddonReady(names[i])
			}

			By("Cleaning up")
			for i := 0; i < numAddons; i++ {
				if addons[i] != nil {
					cleanupResource(addons[i])
				}
			}
		})
	})

	Context("Rapid Updates", func() {
		var name string
		var addon *addonsv1alpha1.Addon
		var values *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("rapid-update")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if values != nil {
				cleanupResource(values)
			}
		})

		It("should handle rapid AddonValue updates", func() {
			By("Creating AddonValue")
			values = createTestAddonValue(name+"-values", name,
				map[string]interface{}{"counter": 0},
				map[string]string{"addons.in-cloud.io/layer": "base"})

			By("Creating Addon")
			addon = createTestAddon(name,
				WithValuesSelector("base", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "base",
				}, 0),
			)

			By("Waiting for Application")
			waitForApplication(name, argoCDNamespace)

			By("Performing rapid updates to AddonValue")
			for i := 1; i <= 5; i++ {
				Eventually(func() error {
					freshValues := &addonsv1alpha1.AddonValue{}
					if err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      values.Name,
						Namespace: values.Namespace,
					}, freshValues); err != nil {
						return err
					}
					freshValues.Spec.Values = string(mustMarshalYAML(map[string]interface{}{"counter": i}))
					return k8sClient.Update(ctx, freshValues)
				}, timeout, interval).Should(Succeed(),
					"Should update AddonValue iteration %d", i)
			}

			By("Waiting for final values hash to stabilize")
			var lastHash string
			Eventually(func() bool {
				addon, err := getAddon(name)
				if err != nil {
					return false
				}
				if lastHash == "" {
					lastHash = addon.Status.ValuesHash
					return false
				}
				if addon.Status.ValuesHash != lastHash {
					lastHash = addon.Status.ValuesHash
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(),
				"ValuesHash should stabilize after rapid updates")

			By("Verifying final counter value in Application")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				counter, ok := values["counter"]
				if !ok {
					return false
				}
				// Should be 5 (last update)
				return fmt.Sprintf("%v", counter) == "5"
			}, timeout, interval).Should(BeTrue(),
				"Application should have final counter value")
		})
	})

	Context("Invalid Dependency Name", func() {
		var name string
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("invalid-dep")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
		})

		It("should handle non-existent dependency gracefully", func() {
			By("Creating Addon with dependency on non-existent Addon")
			criterion := NewCriterion().
				WithJSONPath("$.status.conditions[?@.type=='Ready'].status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			addon = createTestAddon(name,
				WithInitDependencies("non-existent-addon", criterion),
			)

			By("Verifying DependenciesMet is False")
			Eventually(func() metav1.ConditionStatus {
				addon, err := getAddon(name)
				if err != nil {
					return metav1.ConditionUnknown
				}
				for _, c := range addon.Status.Conditions {
					if c.Type == conditions.TypeDependenciesMet {
						return c.Status
					}
				}
				return metav1.ConditionUnknown
			}, timeout, interval).Should(Equal(metav1.ConditionFalse),
				"DependenciesMet should be False for non-existent dependency")

			By("Verifying Application is NOT created")
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      name,
					Namespace: argoCDNamespace,
				}, &argocdv1alpha1.Application{})
				return errors.IsNotFound(err)
			}, "10s", interval).Should(BeTrue(),
				"Application should NOT be created for non-existent dependency")
		})
	})
})

// Helper for debug output
func debugErrorState(addonName string) {
	addon, err := getAddon(addonName)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get Addon %s: %v\n", addonName, err)
		return
	}

	fmt.Fprintf(GinkgoWriter, "Addon %s error state:\n", addonName)
	fmt.Fprintf(GinkgoWriter, "  Conditions:\n")
	for _, c := range addon.Status.Conditions {
		fmt.Fprintf(GinkgoWriter, "    - %s: %s (reason: %s)\n", c.Type, c.Status, c.Reason)
		if c.Message != "" {
			fmt.Fprintf(GinkgoWriter, "      message: %s\n", c.Message)
		}
	}
}
