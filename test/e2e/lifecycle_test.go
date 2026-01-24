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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("Lifecycle & Cleanup", Ordered, func() {
	// Initialize k8s client before all tests
	BeforeAll(func() {
		By("initializing k8s client")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred(), "Failed to initialize k8s client")
		}
	})

	Context("Scenario 6: Cascade Delete (PRD 2.6)", func() {
		// Note: This is THE key test that envtest cannot do!
		// Real Kubernetes garbage collection is required.

		It("should delete Application when Addon is deleted", func() {
			name := uniqueName("cascade-delete")

			By("Creating Addon")
			addon := createTestAddon(name)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying Application has ownerReference to Addon")
			app, err := getApplication(name, argoCDNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(app.OwnerReferences).NotTo(BeEmpty(),
				"Application should have ownerReferences")

			// Find Addon owner reference
			var hasAddonOwner bool
			for _, ref := range app.OwnerReferences {
				if ref.Kind == "Addon" && ref.Name == name {
					hasAddonOwner = true
					break
				}
			}
			Expect(hasAddonOwner).To(BeTrue(),
				"Application should have Addon as owner")

			By("Deleting Addon")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())

			By("Waiting for Addon to be deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &addonsv1alpha1.Addon{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(),
				"Addon should be deleted")

			By("Waiting for Application to be garbage collected")
			// This is the key assertion - real GC should delete the Application
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      name,
					Namespace: argoCDNamespace,
				}, &argocdv1alpha1.Application{})
				return errors.IsNotFound(err)
			}, longTimeout, interval).Should(BeTrue(),
				"Application should be garbage collected after Addon deletion")
		})
	})

	Context("Scenario 7: AddonPhase Cleanup (PRD 2.7)", func() {
		var name string
		var addon *addonsv1alpha1.Addon
		var phase *addonsv1alpha1.AddonPhase
		var alwaysValues *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("phase-cleanup")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			// Phase might already be deleted by the test
			if phase != nil {
				_ = k8sClient.Delete(ctx, phase)
			}
			if alwaysValues != nil {
				cleanupResource(alwaysValues)
			}
			if addon != nil {
				cleanupResource(addon)
			}
		})

		It("should clear phaseValuesSelector when AddonPhase is deleted", func() {
			By("Creating AddonValue for always-active rule")
			alwaysValues = createTestAddonValue(name+"-always", name,
				map[string]interface{}{"phase": map[string]interface{}{"active": true}},
				map[string]string{"always-active": "true"})

			By("Creating Addon")
			addon = createTestAddon(name)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Creating AddonPhase with always-active rule")
			phase = createTestAddonPhase(name,
				WithAlwaysActiveRule(name, "always-on", 50))

			By("Waiting for phaseValuesSelector to be populated")
			waitForPhaseValuesSelector(name, 1)

			By("Verifying phaseValuesSelector contains the rule selector")
			addon, err := getAddon(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(addon.Status.PhaseValuesSelector).To(HaveLen(1))
			Expect(addon.Status.PhaseValuesSelector[0].Name).To(Equal("always-on"))

			By("Deleting AddonPhase")
			Expect(k8sClient.Delete(ctx, phase)).To(Succeed())
			phase = nil // Mark as deleted for cleanup

			By("Waiting for AddonPhase to be deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &addonsv1alpha1.AddonPhase{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(),
				"AddonPhase should be deleted")

			By("Waiting for phaseValuesSelector to be cleared")
			Eventually(func() int {
				addon, err := getAddon(name)
				if err != nil {
					return -1
				}
				return len(addon.Status.PhaseValuesSelector)
			}, timeout, interval).Should(Equal(0),
				"phaseValuesSelector should be empty after AddonPhase deletion")
		})
	})

	Context("Finalizer Behavior", func() {
		It("should have finalizer on Addon", func() {
			name := uniqueName("finalizer-test")

			By("Creating Addon")
			addon := createTestAddon(name)
			defer cleanupResource(addon)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying Addon has finalizer")
			Eventually(func() bool {
				addon, err := getAddon(name)
				if err != nil {
					return false
				}
				for _, f := range addon.Finalizers {
					if f == "addons.in-cloud.io/finalizer" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"Addon should have finalizer")
		})
	})

	Context("Addon Update Triggers Application Update", func() {
		var name string
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("update-test")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
		})

		It("should update Application when Addon spec changes", func() {
			By("Creating Addon with initial version")
			addon = createTestAddon(name)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Getting initial Application spec")
			app, err := getApplication(name, argoCDNamespace)
			Expect(err).NotTo(HaveOccurred())
			initialVersion := app.Spec.Source.TargetRevision

			By("Updating Addon version")
			Eventually(func() error {
				freshAddon, err := getAddon(name)
				if err != nil {
					return err
				}
				freshAddon.Spec.Version = "6.4.0" // Different podinfo version
				return k8sClient.Update(ctx, freshAddon)
			}, timeout, interval).Should(Succeed())

			By("Waiting for Application to be updated")
			Eventually(func() string {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return ""
				}
				return app.Spec.Source.TargetRevision
			}, timeout, interval).ShouldNot(Equal(initialVersion),
				"Application targetRevision should change after Addon update")
		})
	})
})

var _ = Describe("Resource Cleanup Edge Cases", Ordered, func() {
	BeforeAll(func() {
		By("ensuring k8s client is initialized")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Multiple AddonValues Cleanup", func() {
		var name string
		var addon *addonsv1alpha1.Addon
		var values1, values2 *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("multi-values")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if values1 != nil {
				cleanupResource(values1)
			}
			if values2 != nil {
				cleanupResource(values2)
			}
		})

		It("should handle AddonValue deletion without affecting other values", func() {
			By("Creating two AddonValues")
			values1 = createTestAddonValue(name+"-v1", name,
				map[string]interface{}{"layer1": map[string]interface{}{"key": "value1"}},
				map[string]string{"addons.in-cloud.io/layer": "one"})

			values2 = createTestAddonValue(name+"-v2", name,
				map[string]interface{}{"layer2": map[string]interface{}{"key": "value2"}},
				map[string]string{"addons.in-cloud.io/layer": "two"})

			By("Creating Addon with both selectors")
			addon = createTestAddon(name,
				WithValuesSelector("layer-one", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "one",
				}, 10),
				WithValuesSelector("layer-two", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "two",
				}, 20),
			)

			By("Waiting for Application with both values")
			waitForApplication(name, argoCDNamespace)

			By("Verifying both values are in Application")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				_, hasLayer1 := values["layer1"]
				_, hasLayer2 := values["layer2"]
				return hasLayer1 && hasLayer2
			}, timeout, interval).Should(BeTrue(),
				"Application should have both layer values")

			By("Deleting first AddonValue")
			Expect(k8sClient.Delete(ctx, values1)).To(Succeed())
			values1 = nil

			By("Waiting for values hash to change")
			var oldHash string
			Eventually(func() string {
				addon, err := getAddon(name)
				if err != nil {
					return ""
				}
				if oldHash == "" {
					oldHash = addon.Status.ValuesHash
					return "" // Force at least one more iteration
				}
				return addon.Status.ValuesHash
			}, timeout, interval).ShouldNot(Equal(oldHash),
				"ValuesHash should change after AddonValue deletion")

			By("Verifying layer2 value still present, layer1 gone")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				_, hasLayer1 := values["layer1"]
				_, hasLayer2 := values["layer2"]
				return !hasLayer1 && hasLayer2
			}, longTimeout, interval).Should(BeTrue(),
				"Application should only have layer2 value after layer1 AddonValue deletion")
		})
	})

	Context("Addon Recreation", func() {
		It("should handle recreation of Addon with same name", func() {
			name := uniqueName("recreate")

			By("Creating Addon")
			addon := createTestAddon(name)

			By("Waiting for Application")
			waitForApplication(name, argoCDNamespace)

			By("Deleting Addon")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())

			By("Waiting for Addon deletion")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &addonsv1alpha1.Addon{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Waiting for Application deletion")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      name,
					Namespace: argoCDNamespace,
				}, &argocdv1alpha1.Application{})
				return errors.IsNotFound(err)
			}, longTimeout, interval).Should(BeTrue())

			By("Recreating Addon with same name")
			newAddon := createTestAddon(name)
			defer cleanupResource(newAddon)

			By("Waiting for new Application")
			waitForApplication(name, argoCDNamespace)

			By("Verifying new Application is created and syncing")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				// New app should have owner reference to new Addon
				for _, ref := range app.OwnerReferences {
					if ref.Kind == "Addon" && ref.UID == newAddon.UID {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"New Application should have new Addon as owner")
		})
	})
})

// Helper for debug output
func debugLifecycleStatus(addonName string) {
	addon, err := getAddon(addonName)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get Addon %s: %v\n", addonName, err)
		return
	}

	fmt.Fprintf(GinkgoWriter, "Addon %s lifecycle status:\n", addonName)
	fmt.Fprintf(GinkgoWriter, "  Finalizers: %v\n", addon.Finalizers)
	fmt.Fprintf(GinkgoWriter, "  DeletionTimestamp: %v\n", addon.DeletionTimestamp)
	fmt.Fprintf(GinkgoWriter, "  PhaseValuesSelector count: %d\n", len(addon.Status.PhaseValuesSelector))
	fmt.Fprintf(GinkgoWriter, "  Conditions:\n")
	for _, c := range addon.Status.Conditions {
		fmt.Fprintf(GinkgoWriter, "    - %s: %s (%s)\n", c.Type, c.Status, c.Reason)
	}

	if addon.Status.ApplicationRef != nil {
		fmt.Fprintf(GinkgoWriter, "  ApplicationRef: %s/%s\n",
			addon.Status.ApplicationRef.Namespace, addon.Status.ApplicationRef.Name)
	}
}
