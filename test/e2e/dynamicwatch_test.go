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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("Dynamic Watch E2E", Ordered, func() {
	// These tests verify that dynamic watches work correctly in a real cluster.
	// Unlike envtest, the real cluster properly syncs informer caches and triggers
	// reconciles when watched resources change.

	BeforeAll(func() {
		By("initializing k8s client")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred(), "Failed to initialize k8s client")
		}
	})

	Context("Secret Update Triggers Reconcile", func() {
		var name, secretName, addonValueName string
		var addon *addonsv1alpha1.Addon
		var addonValue *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("dw-secret-update")
			secretName = name + "-secret"
			addonValueName = name + "-values"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if addonValue != nil {
				cleanupResource(addonValue)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should update ValuesHash when referenced Secret changes", func() {
			By("Creating Secret with initial data")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"value": []byte("initial-value"),
			})

			By("Creating AddonValue with template that uses extracted values")
			// The template {{ .Values.config.value }} will be rendered with values
			// extracted from the Secret via valuesSources
			addonValue = createTestAddonValue(addonValueName, name, map[string]interface{}{
				"extracted": "{{ .Values.config.value }}",
			}, nil)

			By("Creating Addon with valuesSources and valuesSelectors")
			addon = createTestAddon(name,
				WithValueSource("config", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.value",
						As:       "config.value",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
				WithoutAutoSync(), // Don't need Argo CD sync for this test
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Waiting for ValuesResolved condition")
			waitForCondition(name, "ValuesResolved", metav1.ConditionTrue)

			By("Recording initial ValuesHash")
			var initialHash string
			Eventually(func() string {
				a, err := getAddon(name)
				if err != nil {
					return ""
				}
				initialHash = a.Status.ValuesHash
				return initialHash
			}, timeout, interval).ShouldNot(BeEmpty())

			fmt.Fprintf(GinkgoWriter, "Initial ValuesHash: %s\n", initialHash)

			By("Verifying initial Application values")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				extracted, ok := values["extracted"].(string)
				if !ok {
					fmt.Fprintf(GinkgoWriter, "Values: %+v\n", values)
					return false
				}
				fmt.Fprintf(GinkgoWriter, "Initial extracted value: %s\n", extracted)
				return extracted == "initial-value"
			}, timeout, interval).Should(BeTrue(),
				"Application should have initial values from Secret")

			By("Updating the Secret")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      secretName,
				Namespace: testNamespace,
			}, secret)).To(Succeed())

			secret.Data["value"] = []byte("updated-value")
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			By("Verifying ValuesHash changed (reconcile was triggered)")
			Eventually(func() bool {
				a, err := getAddon(name)
				if err != nil {
					return false
				}
				newHash := a.Status.ValuesHash
				if newHash != "" && newHash != initialHash {
					fmt.Fprintf(GinkgoWriter, "ValuesHash changed: %s -> %s\n", initialHash, newHash)
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"ValuesHash should change when Secret is updated")

			By("Verifying Application has updated values")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				extracted, ok := values["extracted"].(string)
				if !ok {
					return false
				}
				fmt.Fprintf(GinkgoWriter, "Updated extracted value: %s\n", extracted)
				return extracted == "updated-value"
			}, timeout, interval).Should(BeTrue(),
				"Application should have updated values from Secret")
		})
	})

	Context("ConfigMap Update Triggers Reconcile", func() {
		var name, cmName, addonValueName string
		var addon *addonsv1alpha1.Addon
		var addonValue *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("dw-cm-update")
			cmName = name + "-cm"
			addonValueName = name + "-values"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if addonValue != nil {
				cleanupResource(addonValue)
			}
			deleteConfigMap(cmName, testNamespace)
		})

		It("should update ValuesHash when referenced ConfigMap changes", func() {
			By("Creating ConfigMap with initial data")
			createConfigMap(cmName, testNamespace, map[string]string{
				"setting": "initial-setting",
			})

			By("Creating AddonValue with template that uses extracted values")
			addonValue = createTestAddonValue(addonValueName, name, map[string]interface{}{
				"appSetting": "{{ .Values.app.setting }}",
			}, nil)

			By("Creating Addon with valuesSources and valuesSelectors")
			addon = createTestAddon(name,
				WithValueSource("settings", "v1", "ConfigMap", cmName, testNamespace,
					ExtractRule{
						JSONPath: ".data.setting",
						As:       "app.setting",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
				WithoutAutoSync(),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Waiting for ValuesResolved condition")
			waitForCondition(name, "ValuesResolved", metav1.ConditionTrue)

			By("Recording initial ValuesHash")
			var initialHash string
			Eventually(func() string {
				a, err := getAddon(name)
				if err != nil {
					return ""
				}
				initialHash = a.Status.ValuesHash
				return initialHash
			}, timeout, interval).ShouldNot(BeEmpty())

			fmt.Fprintf(GinkgoWriter, "Initial ValuesHash: %s\n", initialHash)

			By("Verifying initial Application values")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				setting, ok := values["appSetting"].(string)
				if !ok {
					fmt.Fprintf(GinkgoWriter, "Values: %+v\n", values)
					return false
				}
				return setting == "initial-setting"
			}, timeout, interval).Should(BeTrue(),
				"Application should have initial setting from ConfigMap")

			By("Updating the ConfigMap")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      cmName,
				Namespace: testNamespace,
			}, cm)).To(Succeed())

			cm.Data["setting"] = "updated-setting"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			By("Verifying ValuesHash changed")
			Eventually(func() bool {
				a, err := getAddon(name)
				if err != nil {
					return false
				}
				newHash := a.Status.ValuesHash
				if newHash != "" && newHash != initialHash {
					fmt.Fprintf(GinkgoWriter, "ValuesHash changed: %s -> %s\n", initialHash, newHash)
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"ValuesHash should change when ConfigMap is updated")

			By("Verifying Application has updated values")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				setting, ok := values["appSetting"].(string)
				if !ok {
					return false
				}
				fmt.Fprintf(GinkgoWriter, "Updated appSetting: %s\n", setting)
				return setting == "updated-setting"
			}, timeout, interval).Should(BeTrue(),
				"Application should have updated setting from ConfigMap")
		})
	})

	Context("Multiple Addons Sharing Watch", func() {
		var addon1Name, addon2Name, secretName, av1Name, av2Name string
		var addon1, addon2 *addonsv1alpha1.Addon
		var addonValue1, addonValue2 *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			baseName := uniqueName("dw-shared")
			addon1Name = baseName + "-1"
			addon2Name = baseName + "-2"
			secretName = baseName + "-secret"
			av1Name = addon1Name + "-values"
			av2Name = addon2Name + "-values"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon1 != nil {
				cleanupResource(addon1)
			}
			if addon2 != nil {
				cleanupResource(addon2)
			}
			if addonValue1 != nil {
				cleanupResource(addonValue1)
			}
			if addonValue2 != nil {
				cleanupResource(addonValue2)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should trigger reconcile for both Addons when shared Secret changes", func() {
			By("Creating shared Secret")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"shared": []byte("initial-shared"),
			})

			By("Creating AddonValues with templates")
			addonValue1 = createTestAddonValue(av1Name, addon1Name, map[string]interface{}{
				"sharedValue": "{{ .Values.config.shared }}",
			}, nil)
			addonValue2 = createTestAddonValue(av2Name, addon2Name, map[string]interface{}{
				"sharedValue": "{{ .Values.settings.shared }}",
			}, nil)

			By("Creating first Addon referencing Secret")
			addon1 = createTestAddon(addon1Name,
				WithValueSource("shared", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.shared",
						As:       "config.shared",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": addon1Name,
				}, 0),
				WithoutAutoSync(),
			)

			By("Creating second Addon referencing same Secret")
			addon2 = createTestAddon(addon2Name,
				WithValueSource("shared", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.shared",
						As:       "settings.shared",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": addon2Name,
				}, 0),
				WithoutAutoSync(),
			)

			By("Waiting for both Applications")
			waitForApplication(addon1Name, argoCDNamespace)
			waitForApplication(addon2Name, argoCDNamespace)

			By("Waiting for both Addons to have ValuesResolved")
			waitForCondition(addon1Name, "ValuesResolved", metav1.ConditionTrue)
			waitForCondition(addon2Name, "ValuesResolved", metav1.ConditionTrue)

			By("Recording initial ValuesHashes")
			var hash1, hash2 string
			Eventually(func() bool {
				a1, err := getAddon(addon1Name)
				if err != nil {
					return false
				}
				a2, err := getAddon(addon2Name)
				if err != nil {
					return false
				}
				hash1 = a1.Status.ValuesHash
				hash2 = a2.Status.ValuesHash
				return hash1 != "" && hash2 != ""
			}, timeout, interval).Should(BeTrue())

			fmt.Fprintf(GinkgoWriter, "Initial hashes: addon1=%s, addon2=%s\n", hash1, hash2)

			By("Updating the shared Secret")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      secretName,
				Namespace: testNamespace,
			}, secret)).To(Succeed())

			secret.Data["shared"] = []byte("updated-shared")
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			By("Verifying BOTH Addons have new ValuesHash")
			Eventually(func() bool {
				a1, err := getAddon(addon1Name)
				if err != nil {
					return false
				}
				a2, err := getAddon(addon2Name)
				if err != nil {
					return false
				}

				newHash1 := a1.Status.ValuesHash
				newHash2 := a2.Status.ValuesHash

				hash1Changed := newHash1 != "" && newHash1 != hash1
				hash2Changed := newHash2 != "" && newHash2 != hash2

				if hash1Changed {
					fmt.Fprintf(GinkgoWriter, "Addon1 hash changed: %s -> %s\n", hash1, newHash1)
				}
				if hash2Changed {
					fmt.Fprintf(GinkgoWriter, "Addon2 hash changed: %s -> %s\n", hash2, newHash2)
				}

				return hash1Changed && hash2Changed
			}, timeout, interval).Should(BeTrue(),
				"Both Addons should have updated ValuesHash after shared Secret changes")
		})
	})

	Context("Watch Cleanup on Addon Deletion", func() {
		var addon1Name, addon2Name, secretName, av1Name, av2Name string
		var addon1, addon2 *addonsv1alpha1.Addon
		var addonValue1, addonValue2 *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			baseName := uniqueName("dw-cleanup")
			addon1Name = baseName + "-1"
			addon2Name = baseName + "-2"
			secretName = baseName + "-secret"
			av1Name = addon1Name + "-values"
			av2Name = addon2Name + "-values"
		})

		AfterEach(func() {
			By("cleaning up remaining resources")
			if addon2 != nil {
				cleanupResource(addon2)
			}
			if addonValue1 != nil {
				cleanupResource(addonValue1)
			}
			if addonValue2 != nil {
				cleanupResource(addonValue2)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should continue watching for remaining Addon after one is deleted", func() {
			By("Creating shared Secret")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"data": []byte("test-data"),
			})

			By("Creating AddonValues with templates")
			addonValue1 = createTestAddonValue(av1Name, addon1Name, map[string]interface{}{
				"secretData": "{{ .Values.config.data }}",
			}, nil)
			addonValue2 = createTestAddonValue(av2Name, addon2Name, map[string]interface{}{
				"secretData": "{{ .Values.settings.data }}",
			}, nil)

			By("Creating two Addons referencing Secret")
			addon1 = createTestAddon(addon1Name,
				WithValueSource("config", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.data",
						As:       "config.data",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": addon1Name,
				}, 0),
				WithoutAutoSync(),
			)

			addon2 = createTestAddon(addon2Name,
				WithValueSource("config", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.data",
						As:       "settings.data",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": addon2Name,
				}, 0),
				WithoutAutoSync(),
			)

			By("Waiting for both Addons to be ready")
			waitForApplication(addon1Name, argoCDNamespace)
			waitForApplication(addon2Name, argoCDNamespace)
			waitForCondition(addon1Name, "ValuesResolved", metav1.ConditionTrue)
			waitForCondition(addon2Name, "ValuesResolved", metav1.ConditionTrue)

			By("Deleting first Addon")
			cleanupResource(addon1)
			addon1 = nil

			// Small delay to ensure deletion is processed
			time.Sleep(2 * time.Second)

			By("Recording second Addon's hash")
			var hash2 string
			Eventually(func() string {
				a, err := getAddon(addon2Name)
				if err != nil {
					return ""
				}
				hash2 = a.Status.ValuesHash
				return hash2
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Updating Secret (should still trigger reconcile for addon2)")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      secretName,
				Namespace: testNamespace,
			}, secret)).To(Succeed())

			secret.Data["data"] = []byte("new-data-after-delete")
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			By("Verifying second Addon still gets reconciled")
			Eventually(func() bool {
				a, err := getAddon(addon2Name)
				if err != nil {
					return false
				}
				newHash := a.Status.ValuesHash
				if newHash != "" && newHash != hash2 {
					fmt.Fprintf(GinkgoWriter, "Addon2 hash changed after addon1 deletion: %s -> %s\n",
						hash2, newHash)
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"Remaining Addon should still be reconciled when Secret changes")
		})
	})

	Context("Graceful Degradation for Missing Source", func() {
		var name, secretName, addonValueName string
		var addon *addonsv1alpha1.Addon
		var addonValue *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("dw-missing")
			secretName = name + "-secret"
			addonValueName = name + "-values"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if addonValue != nil {
				cleanupResource(addonValue)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should recover when missing Secret is created", func() {
			By("Creating AddonValue with template")
			addonValue = createTestAddonValue(addonValueName, name, map[string]interface{}{
				"secretKey": "{{ .Values.config.key }}",
			}, nil)

			By("Creating Addon referencing non-existent Secret")
			addon = createTestAddon(name,
				WithValueSource("config", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.key",
						As:       "config.key",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
				WithoutAutoSync(),
			)

			By("Verifying Addon reports ValueSourceError")
			waitForConditionReason(name, "ValuesResolved", "ValueSourceError")

			By("Creating the missing Secret")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"key": []byte("secret-value"),
			})

			By("Verifying Addon recovers")
			waitForCondition(name, "ValuesResolved", metav1.ConditionTrue)

			By("Verifying Application is created with correct values")
			waitForApplication(name, argoCDNamespace)

			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				secretKey, ok := values["secretKey"].(string)
				if !ok {
					fmt.Fprintf(GinkgoWriter, "Values: %+v\n", values)
					return false
				}
				return secretKey == "secret-value"
			}, timeout, interval).Should(BeTrue(),
				"Application should have values from newly created Secret")
		})
	})

	Context("SourceRef Change", func() {
		var name, secret1Name, secret2Name, addonValueName string
		var addon *addonsv1alpha1.Addon
		var addonValue *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("dw-sourceref-change")
			secret1Name = name + "-secret1"
			secret2Name = name + "-secret2"
			addonValueName = name + "-values"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if addonValue != nil {
				cleanupResource(addonValue)
			}
			deleteTestSecret(secret1Name, testNamespace)
			deleteTestSecret(secret2Name, testNamespace)
		})

		It("should switch watches when SourceRef changes", func() {
			By("Creating two Secrets with different values")
			createTestSecret(secret1Name, testNamespace, map[string][]byte{
				"value": []byte("from-secret-1"),
			})
			createTestSecret(secret2Name, testNamespace, map[string][]byte{
				"value": []byte("from-secret-2"),
			})

			By("Creating AddonValue with template")
			addonValue = createTestAddonValue(addonValueName, name, map[string]interface{}{
				"extractedValue": "{{ .Values.config.value }}",
			}, nil)

			By("Creating Addon referencing first Secret")
			addon = createTestAddon(name,
				WithValueSource("config", "v1", "Secret", secret1Name, testNamespace,
					ExtractRule{
						JSONPath: ".data.value",
						As:       "config.value",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
				WithoutAutoSync(),
			)

			By("Waiting for initial reconcile")
			waitForApplication(name, argoCDNamespace)
			waitForCondition(name, "ValuesResolved", metav1.ConditionTrue)

			By("Verifying value from first Secret")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				v, ok := values["extractedValue"].(string)
				if !ok {
					fmt.Fprintf(GinkgoWriter, "Values: %+v\n", values)
					return false
				}
				return v == "from-secret-1"
			}, timeout, interval).Should(BeTrue())

			By("Changing SourceRef to second Secret")
			Eventually(func() error {
				a, err := getAddon(name)
				if err != nil {
					return err
				}
				a.Spec.ValuesSources[0].SourceRef.Name = secret2Name
				return k8sClient.Update(ctx, a)
			}, timeout, interval).Should(Succeed())

			By("Verifying value from second Secret")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				v, ok := values["extractedValue"].(string)
				if !ok {
					return false
				}
				fmt.Fprintf(GinkgoWriter, "Current value: %v\n", v)
				return v == "from-secret-2"
			}, timeout, interval).Should(BeTrue(),
				"Application should have values from second Secret after SourceRef change")

			By("Verifying updates to second Secret trigger reconcile")
			var hashBeforeUpdate string
			Eventually(func() string {
				a, err := getAddon(name)
				if err != nil {
					return ""
				}
				hashBeforeUpdate = a.Status.ValuesHash
				return hashBeforeUpdate
			}, timeout, interval).ShouldNot(BeEmpty())

			secret2 := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      secret2Name,
				Namespace: testNamespace,
			}, secret2)).To(Succeed())
			secret2.Data["value"] = []byte("updated-secret-2")
			Expect(k8sClient.Update(ctx, secret2)).To(Succeed())

			Eventually(func() bool {
				a, err := getAddon(name)
				if err != nil {
					return false
				}
				newHash := a.Status.ValuesHash
				if newHash != "" && newHash != hashBeforeUpdate {
					fmt.Fprintf(GinkgoWriter, "Hash changed: %s -> %s\n", hashBeforeUpdate, newHash)
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"ValuesHash should change after updating the new watched Secret")
		})
	})

	Context("Mixed Source Types", func() {
		var name, secretName, cmName, addonValueName string
		var addon *addonsv1alpha1.Addon
		var addonValue *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("dw-mixed")
			secretName = name + "-secret"
			cmName = name + "-cm"
			addonValueName = name + "-values"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if addonValue != nil {
				cleanupResource(addonValue)
			}
			deleteTestSecret(secretName, testNamespace)
			deleteConfigMap(cmName, testNamespace)
		})

		It("should watch both Secret and ConfigMap and reconcile on either change", func() {
			By("Creating Secret and ConfigMap")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"secret-key": []byte("secret-value"),
			})
			createConfigMap(cmName, testNamespace, map[string]string{
				"cm-key": "cm-value",
			})

			By("Creating AddonValue with templates for both sources")
			addonValue = createTestAddonValue(addonValueName, name, map[string]interface{}{
				"fromSecret": "{{ .Values.config.secret }}",
				"fromCM":     "{{ .Values.config.cm }}",
			}, nil)

			By("Creating Addon with both sources")
			addon = createTestAddon(name,
				WithValueSource("secret-src", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.secret-key",
						As:       "config.secret",
						Decode:   "base64",
					}),
				WithValueSource("cm-src", "v1", "ConfigMap", cmName, testNamespace,
					ExtractRule{
						JSONPath: ".data.cm-key",
						As:       "config.cm",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
				WithoutAutoSync(),
			)

			By("Waiting for initial reconcile")
			waitForApplication(name, argoCDNamespace)
			waitForCondition(name, "ValuesResolved", metav1.ConditionTrue)

			var hash1 string
			Eventually(func() string {
				a, err := getAddon(name)
				if err != nil {
					return ""
				}
				hash1 = a.Status.ValuesHash
				return hash1
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Verifying initial values")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				fromSecret, ok1 := values["fromSecret"].(string)
				fromCM, ok2 := values["fromCM"].(string)
				if !ok1 || !ok2 {
					fmt.Fprintf(GinkgoWriter, "Values: %+v\n", values)
					return false
				}
				return fromSecret == "secret-value" && fromCM == "cm-value"
			}, timeout, interval).Should(BeTrue(),
				"Application should have initial values from both sources")

			By("Updating Secret - should trigger reconcile")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      secretName,
				Namespace: testNamespace,
			}, secret)).To(Succeed())
			secret.Data["secret-key"] = []byte("updated-secret")
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			var hash2 string
			Eventually(func() bool {
				a, err := getAddon(name)
				if err != nil {
					return false
				}
				hash2 = a.Status.ValuesHash
				if hash2 != "" && hash2 != hash1 {
					fmt.Fprintf(GinkgoWriter, "Hash after Secret update: %s -> %s\n", hash1, hash2)
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"ValuesHash should change after Secret update")

			By("Verifying values updated from Secret")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				fromSecret, ok := values["fromSecret"].(string)
				if !ok {
					return false
				}
				return fromSecret == "updated-secret"
			}, timeout, interval).Should(BeTrue(),
				"Application should have updated value from Secret")

			By("Updating ConfigMap - should also trigger reconcile")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      cmName,
				Namespace: testNamespace,
			}, cm)).To(Succeed())
			cm.Data["cm-key"] = "updated-cm"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			Eventually(func() bool {
				a, err := getAddon(name)
				if err != nil {
					return false
				}
				hash3 := a.Status.ValuesHash
				if hash3 != "" && hash3 != hash2 {
					fmt.Fprintf(GinkgoWriter, "Hash after CM update: %s -> %s\n", hash2, hash3)
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"ValuesHash should change after ConfigMap update")

			By("Verifying values updated from ConfigMap")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				fromCM, ok := values["fromCM"].(string)
				if !ok {
					return false
				}
				return fromCM == "updated-cm"
			}, timeout, interval).Should(BeTrue(),
				"Application should have updated value from ConfigMap")
		})
	})
})

// createConfigMap creates a ConfigMap for testing.
func createConfigMap(name, namespace string, data map[string]string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
	Expect(k8sClient.Create(ctx, cm)).To(Succeed(), "Failed to create ConfigMap %s/%s", namespace, name)
	return cm
}

// deleteConfigMap deletes a ConfigMap.
func deleteConfigMap(name, namespace string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	_ = k8sClient.Delete(ctx, cm)
}
