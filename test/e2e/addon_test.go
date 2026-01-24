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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/conditions"
)

var _ = Describe("Basic Addon Deployment", Ordered, func() {
	// Initialize k8s client before all tests
	BeforeAll(func() {
		By("initializing k8s client")
		err := initK8sClient()
		Expect(err).NotTo(HaveOccurred(), "Failed to initialize k8s client")
	})

	Context("Scenario 1: Basic Addon Deployment (PRD 2.1)", func() {
		var name string

		BeforeEach(func() {
			name = uniqueName("basic-addon")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			// Clean up Addon (will cascade delete Application)
			addon := &addonsv1alpha1.Addon{}
			addon.Name = name
			cleanupResource(addon)

			// Clean up AddonValue
			av := &addonsv1alpha1.AddonValue{}
			av.Name = name + "-values"
			cleanupResource(av)
		})

		It("should create Application and sync successfully", func() {
			By("Creating AddonValue")
			createTestAddonValue(name+"-values", name,
				map[string]interface{}{"replicaCount": 1},
				map[string]string{"addons.in-cloud.io/layer": "default"},
			)

			By("Creating Addon")
			createTestAddon(name,
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "default",
				}, 0),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Waiting for Application to sync")
			waitForApplicationSynced(name, argoCDNamespace)

			By("Verifying Addon conditions")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
			waitForCondition(name, conditions.TypeSynced, metav1.ConditionTrue)

			By("Verifying ApplicationRef in status")
			Eventually(func() string {
				addon, err := getAddon(name)
				if err != nil {
					return ""
				}
				if addon.Status.ApplicationRef == nil {
					return ""
				}
				return addon.Status.ApplicationRef.Name
			}, timeout, interval).Should(Equal(name), "ApplicationRef.Name should match Addon name")
		})
	})

	Context("Status Translation", func() {
		var name string

		BeforeEach(func() {
			name = uniqueName("status-test")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = name
			cleanupResource(addon)
		})

		It("should translate Application sync/health to Addon conditions", func() {
			By("Creating Addon")
			createTestAddon(name)

			By("Waiting for Application to exist")
			waitForApplication(name, argoCDNamespace)

			By("Verifying Synced condition reflects Application sync")
			Eventually(func() bool {
				addon, err := getAddon(name)
				if err != nil {
					return false
				}
				for _, c := range addon.Status.Conditions {
					if c.Type == conditions.TypeSynced {
						return true
					}
				}
				return false
			}, longTimeout, interval).Should(BeTrue(), "Addon should have Synced condition")

			By("Verifying ApplicationCreated condition is True")
			waitForCondition(name, conditions.TypeApplicationCreated, metav1.ConditionTrue)
		})
	})

	Context("ApplicationRef Population", func() {
		var name string

		BeforeEach(func() {
			name = uniqueName("appref-test")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = name
			cleanupResource(addon)
		})

		It("should populate ApplicationRef in Addon status", func() {
			By("Creating Addon")
			createTestAddon(name)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying ApplicationRef is populated")
			Eventually(func() bool {
				addon, err := getAddon(name)
				if err != nil {
					return false
				}
				return addon.Status.ApplicationRef != nil &&
					addon.Status.ApplicationRef.Name != "" &&
					addon.Status.ApplicationRef.Namespace != ""
			}, timeout, interval).Should(BeTrue(), "ApplicationRef should be populated")

			By("Verifying ApplicationRef values")
			addon, err := getAddon(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(addon.Status.ApplicationRef.Name).To(Equal(name))
			Expect(addon.Status.ApplicationRef.Namespace).To(Equal(argoCDNamespace))
		})
	})

	Context("ValuesHash Update", func() {
		var name string

		BeforeEach(func() {
			name = uniqueName("hash-test")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = name
			cleanupResource(addon)

			av := &addonsv1alpha1.AddonValue{}
			av.Name = name + "-values"
			cleanupResource(av)
		})

		It("should update ValuesHash when AddonValue changes", func() {
			By("Creating AddonValue")
			av := createTestAddonValue(name+"-values", name,
				map[string]interface{}{"key": "value1"},
				nil,
			)

			By("Creating Addon")
			createTestAddon(name,
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
			)

			By("Waiting for initial ValuesHash")
			var initialHash string
			Eventually(func() string {
				addon, err := getAddon(name)
				if err != nil {
					return ""
				}
				initialHash = addon.Status.ValuesHash
				return initialHash
			}, timeout, interval).ShouldNot(BeEmpty(), "ValuesHash should be populated")

			By("Updating AddonValue")
			Eventually(func() error {
				// Get fresh copy
				freshAV := &addonsv1alpha1.AddonValue{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: av.Name}, freshAV); err != nil {
					return err
				}
				// Update values
				freshAV.Spec.Values.Raw = mustMarshal(map[string]interface{}{"key": "value2"})
				return k8sClient.Update(ctx, freshAV)
			}, timeout, interval).Should(Succeed(), "Should update AddonValue")

			By("Verifying ValuesHash changed")
			Eventually(func() bool {
				addon, err := getAddon(name)
				if err != nil {
					return false
				}
				// Hash should be different and not empty
				return addon.Status.ValuesHash != "" && addon.Status.ValuesHash != initialHash
			}, timeout, interval).Should(BeTrue(), "ValuesHash should change after AddonValue update")
		})
	})

	Context("Addon Phase Progression", func() {
		var name string

		BeforeEach(func() {
			name = uniqueName("phase-test")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = name
			cleanupResource(addon)
		})

		It("should progress through phases to Ready", func() {
			By("Creating Addon")
			createTestAddon(name)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Waiting for Addon to become Ready")
			waitForAddonReady(name)
		})
	})
})

// Describe block for Addon deletion tests
var _ = Describe("Addon Deletion", Ordered, func() {
	BeforeAll(func() {
		By("ensuring k8s client is initialized")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Addon Cleanup", func() {
		It("should delete Addon successfully", func() {
			name := uniqueName("delete-test")

			By("Creating Addon")
			addon := createTestAddon(name)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Deleting Addon")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())

			By("Waiting for Addon to be deleted")
			waitForDeletion(addon)

			By("Verifying Application is also deleted (cascade)")
			// Note: In real Kind cluster, garbage collection will delete the Application
			// This may take some time due to GC timing
			Eventually(func() bool {
				_, err := getApplication(name, argoCDNamespace)
				return err != nil // Application should not exist
			}, longTimeout, interval).Should(BeTrue(),
				"Application should be deleted when Addon is deleted")
		})
	})
})

// Helper for debug output
func debugAddonStatus(name string) {
	addon, err := getAddon(name)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get addon %s: %v\n", name, err)
		return
	}
	fmt.Fprintf(GinkgoWriter, "Addon %s status:\n", name)
	fmt.Fprintf(GinkgoWriter, "  ValuesHash: %s\n", addon.Status.ValuesHash)
	if addon.Status.ApplicationRef != nil {
		fmt.Fprintf(GinkgoWriter, "  ApplicationRef: %s/%s\n",
			addon.Status.ApplicationRef.Namespace, addon.Status.ApplicationRef.Name)
	}
	fmt.Fprintf(GinkgoWriter, "  Conditions:\n")
	for _, c := range addon.Status.Conditions {
		fmt.Fprintf(GinkgoWriter, "    - %s: %s (%s)\n", c.Type, c.Status, c.Reason)
	}
}
