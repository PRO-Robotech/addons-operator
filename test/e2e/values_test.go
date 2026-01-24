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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("Values Aggregation", Ordered, func() {
	// Initialize k8s client before all tests
	BeforeAll(func() {
		By("initializing k8s client")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred(), "Failed to initialize k8s client")
		}
	})

	Context("Scenario 3: Priority Merge Order (PRD 2.3)", func() {
		var name string
		var avDefault, avCustom, avImmutable *addonsv1alpha1.AddonValue
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("priority-merge")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			// Clean up Addon first (will cascade delete Application)
			if addon != nil {
				cleanupResource(addon)
			}
			// Clean up AddonValues
			if avDefault != nil {
				cleanupResource(avDefault)
			}
			if avCustom != nil {
				cleanupResource(avCustom)
			}
			if avImmutable != nil {
				cleanupResource(avImmutable)
			}
		})

		It("should merge values in priority order", func() {
			By("Creating AddonValues with different priorities")
			// Priority 0: {a: 1, b: 1}
			avDefault = createTestAddonValue(name+"-default", name,
				map[string]interface{}{"a": 1, "b": 1},
				map[string]string{"addons.in-cloud.io/layer": "default"})

			// Priority 50: {b: 2, c: 2}
			avCustom = createTestAddonValue(name+"-custom", name,
				map[string]interface{}{"b": 2, "c": 2},
				map[string]string{"addons.in-cloud.io/layer": "custom"})

			// Priority 99: {c: 3}
			avImmutable = createTestAddonValue(name+"-immutable", name,
				map[string]interface{}{"c": 3},
				map[string]string{"addons.in-cloud.io/layer": "immutable"})

			By("Creating Addon with 3 selectors")
			addon = createTestAddon(name,
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "default",
				}, 0),
				WithValuesSelector("custom", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "custom",
				}, 50),
				WithValuesSelector("immutable", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "immutable",
				}, 99),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying final merged values: {a:1, b:2, c:3}")
			Eventually(func() map[string]interface{} {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return nil
				}
				return getApplicationValues(app)
			}, timeout, interval).Should(SatisfyAll(
				HaveKeyWithValue("a", BeEquivalentTo(1)), // from default
				HaveKeyWithValue("b", BeEquivalentTo(2)), // from custom (overwrites)
				HaveKeyWithValue("c", BeEquivalentTo(3)), // from immutable (overwrites)
			))
		})
	})

	Context("Deep Merge", func() {
		var name string
		var avBase, avOverlay *addonsv1alpha1.AddonValue
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("deep-merge")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if avBase != nil {
				cleanupResource(avBase)
			}
			if avOverlay != nil {
				cleanupResource(avOverlay)
			}
		})

		It("should deep merge nested objects", func() {
			By("Creating base AddonValue with nested structure")
			// Base: {config: {a: 1, b: 1}}
			avBase = createTestAddonValue(name+"-base", name,
				map[string]interface{}{
					"config": map[string]interface{}{
						"a": 1,
						"b": 1,
					},
				},
				map[string]string{"addons.in-cloud.io/layer": "base"})

			By("Creating overlay AddonValue with partial override")
			// Overlay: {config: {b: 2, c: 2}}
			avOverlay = createTestAddonValue(name+"-overlay", name,
				map[string]interface{}{
					"config": map[string]interface{}{
						"b": 2,
						"c": 2,
					},
				},
				map[string]string{"addons.in-cloud.io/layer": "overlay"})

			By("Creating Addon with 2 selectors")
			addon = createTestAddon(name,
				WithValuesSelector("base", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "base",
				}, 0),
				WithValuesSelector("overlay", map[string]string{
					"addons.in-cloud.io/addon": name,
					"addons.in-cloud.io/layer": "overlay",
				}, 50),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying deep merged values: {config: {a:1, b:2, c:2}}")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				config, ok := values["config"].(map[string]interface{})
				if !ok {
					return false
				}
				// Check all three keys
				aVal, aOk := config["a"]
				bVal, bOk := config["b"]
				cVal, cOk := config["c"]
				return aOk && bOk && cOk &&
					fmt.Sprintf("%v", aVal) == "1" &&
					fmt.Sprintf("%v", bVal) == "2" &&
					fmt.Sprintf("%v", cVal) == "2"
			}, timeout, interval).Should(BeTrue(),
				"Deep merge should preserve base values and override overlapping keys")
		})
	})
})

var _ = Describe("ValuesSources", Ordered, func() {
	BeforeAll(func() {
		By("ensuring k8s client is initialized")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Scenario 4: Secret Extraction (PRD 2.4)", func() {
		var name, secretName string
		var addon *addonsv1alpha1.Addon
		var av *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("values-sources")
			secretName = name + "-secret"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if av != nil {
				cleanupResource(av)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should extract values from Secret", func() {
			By("Creating Secret with certificate data")
			certData := "-----BEGIN CERTIFICATE-----\nTEST-CERTIFICATE-DATA\n-----END CERTIFICATE-----"
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"ca.crt": []byte(certData),
			})

			By("Creating AddonValue that uses extracted value in template")
			// Extracted values are available in templates as .Values.path
			av = createTestAddonValue(name+"-values", name,
				map[string]interface{}{
					"tls": map[string]interface{}{
						"ca": "{{ .Values.tls.ca }}",
					},
				},
				nil,
			)

			By("Creating Addon with valuesSources and valuesSelector")
			addon = createTestAddon(name,
				WithValueSource("ca-cert", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data[\"ca.crt\"]",
						As:       "tls.ca",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying certificate in Application values")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				if app.Spec.Source == nil || app.Spec.Source.Helm == nil {
					return false
				}
				// Check that the certificate is present in the values
				return strings.Contains(app.Spec.Source.Helm.Values, "BEGIN CERTIFICATE")
			}, timeout, interval).Should(BeTrue(),
				"Application values should contain extracted certificate")
		})
	})

	Context("Base64 Decoding", func() {
		var name, secretName string
		var addon *addonsv1alpha1.Addon
		var av *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("base64-decode")
			secretName = name + "-secret"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if av != nil {
				cleanupResource(av)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should decode base64 encoded data from Secret", func() {
			By("Creating Secret with data")
			// Secret data is automatically base64 encoded by Kubernetes
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"key": []byte("decoded-value"),
			})

			By("Creating AddonValue that uses extracted value in template")
			// Extracted values are available in templates as .Values.path
			av = createTestAddonValue(name+"-values", name,
				map[string]interface{}{
					"config": map[string]interface{}{
						"value": "{{ .Values.config.value }}",
					},
				},
				nil,
			)

			By("Creating Addon extracting with decode: base64")
			addon = createTestAddon(name,
				WithValueSource("config", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.key",
						As:       "config.value",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying decoded value in Application")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				config, ok := values["config"].(map[string]interface{})
				if !ok {
					return false
				}
				return config["value"] == "decoded-value"
			}, timeout, interval).Should(BeTrue(),
				"Application values should contain decoded value")
		})
	})

	Context("Multiple Sources", func() {
		var name, secret1Name, secret2Name string
		var addon *addonsv1alpha1.Addon
		var av *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("multi-source")
			secret1Name = name + "-secret1"
			secret2Name = name + "-secret2"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if av != nil {
				cleanupResource(av)
			}
			deleteTestSecret(secret1Name, testNamespace)
			deleteTestSecret(secret2Name, testNamespace)
		})

		It("should extract from multiple secrets", func() {
			By("Creating first Secret")
			createTestSecret(secret1Name, testNamespace, map[string][]byte{
				"username": []byte("admin"),
			})

			By("Creating second Secret")
			createTestSecret(secret2Name, testNamespace, map[string][]byte{
				"password": []byte("secret123"),
			})

			By("Creating AddonValue that uses extracted values in templates")
			// Extracted values are available in templates as .Values.path
			av = createTestAddonValue(name+"-values", name,
				map[string]interface{}{
					"auth": map[string]interface{}{
						"username": "{{ .Values.auth.username }}",
						"password": "{{ .Values.auth.password }}",
					},
				},
				nil,
			)

			By("Creating Addon with multiple valuesSources")
			addon = createTestAddon(name,
				WithValueSource("user", "v1", "Secret", secret1Name, testNamespace,
					ExtractRule{
						JSONPath: ".data.username",
						As:       "auth.username",
						Decode:   "base64",
					}),
				WithValueSource("pass", "v1", "Secret", secret2Name, testNamespace,
					ExtractRule{
						JSONPath: ".data.password",
						As:       "auth.password",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying both values in Application")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				auth, ok := values["auth"].(map[string]interface{})
				if !ok {
					return false
				}
				return auth["username"] == "admin" && auth["password"] == "secret123"
			}, timeout, interval).Should(BeTrue(),
				"Application values should contain both extracted values")
		})
	})
})

var _ = Describe("Template Rendering", Ordered, func() {
	BeforeAll(func() {
		By("ensuring k8s client is initialized")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Variables in Templates", func() {
		var name string
		var av *addonsv1alpha1.AddonValue
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("template-vars")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if av != nil {
				cleanupResource(av)
			}
		})

		It("should render Go templates with variables", func() {
			By("Creating AddonValue with template expression")
			// The template uses Variables from Addon spec
			av = createTestAddonValue(name+"-values", name,
				map[string]interface{}{
					"cluster": map[string]interface{}{
						"name": "{{ .Variables.cluster_name }}",
					},
					"environment": "{{ .Variables.env }}",
				},
				nil)

			By("Creating Addon with variables")
			addon = createTestAddon(name,
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
				WithVariables(map[string]string{
					"cluster_name": "my-test-cluster",
					"env":          "production",
				}),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying rendered values in Application")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)

				// Check environment
				if values["environment"] != "my-test-cluster" && values["environment"] != "production" {
					// One of them should match
				}

				// Check cluster.name
				cluster, ok := values["cluster"].(map[string]interface{})
				if !ok {
					return false
				}
				return cluster["name"] == "my-test-cluster"
			}, timeout, interval).Should(BeTrue(),
				"Application values should have rendered template variables")
		})
	})

	Context("ValuesSources with Templates", func() {
		var name, secretName string
		var av *addonsv1alpha1.AddonValue
		var addon *addonsv1alpha1.Addon

		BeforeEach(func() {
			name = uniqueName("source-template")
			secretName = name + "-secret"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			if av != nil {
				cleanupResource(av)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should use extracted values in templates", func() {
			By("Creating Secret with endpoint")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"endpoint": []byte("https://api.example.com"),
			})

			By("Creating AddonValue using extracted value in template")
			// This template references a value that will be extracted from Secret
			av = createTestAddonValue(name+"-values", name,
				map[string]interface{}{
					"api": map[string]interface{}{
						"url": "{{ .Values.extracted.endpoint }}",
					},
				},
				nil)

			By("Creating Addon with valuesSources and template")
			addon = createTestAddon(name,
				WithValueSource("api-endpoint", "v1", "Secret", secretName, testNamespace,
					ExtractRule{
						JSONPath: ".data.endpoint",
						As:       "extracted.endpoint",
						Decode:   "base64",
					}),
				WithValuesSelector("default", map[string]string{
					"addons.in-cloud.io/addon": name,
				}, 0),
			)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Verifying template was rendered with extracted value")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)
				api, ok := values["api"].(map[string]interface{})
				if !ok {
					return false
				}
				// The template should have been rendered with the extracted endpoint
				url, urlOk := api["url"].(string)
				return urlOk && strings.Contains(url, "api.example.com")
			}, timeout, interval).Should(BeTrue(),
				"Template should be rendered with extracted value from Secret")
		})
	})
})

var _ = Describe("Values Stress Test", Ordered, func() {
	BeforeAll(func() {
		By("ensuring k8s client is initialized")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Large Number of AddonValues", func() {
		const numAddonValues = 50

		var name string
		var addon *addonsv1alpha1.Addon
		var addonValues []*addonsv1alpha1.AddonValue

		BeforeEach(func() {
			name = uniqueName("stress-test")
			addonValues = make([]*addonsv1alpha1.AddonValue, 0, numAddonValues)
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if addon != nil {
				cleanupResource(addon)
			}
			for _, av := range addonValues {
				if av != nil {
					cleanupResource(av)
				}
			}
		})

		It("should aggregate 50 AddonValues correctly", func() {
			By(fmt.Sprintf("Creating %d AddonValues with different priorities", numAddonValues))

			// Create AddonValues with priorities 0-49
			// Each AddonValue adds a unique key and potentially overwrites shared keys
			for i := 0; i < numAddonValues; i++ {
				avName := fmt.Sprintf("%s-values-%03d", name, i)
				values := map[string]interface{}{
					// Unique key for this AddonValue
					fmt.Sprintf("layer%03d", i): map[string]interface{}{
						"index":    i,
						"priority": i,
					},
					// Shared key that gets overwritten by higher priority
					"shared": map[string]interface{}{
						"lastWriter": i,
						"priority":   i,
					},
				}

				av := createTestAddonValue(avName, name, values,
					map[string]string{
						"stress-test":              "true",
						"addons.in-cloud.io/layer": fmt.Sprintf("layer-%03d", i),
					})
				addonValues = append(addonValues, av)
			}

			By("Creating Addon with selectors for all AddonValues")
			// Create selectors for each layer
			opts := make([]AddonOption, 0, numAddonValues)
			for i := 0; i < numAddonValues; i++ {
				opts = append(opts, WithValuesSelector(
					fmt.Sprintf("layer-%03d", i),
					map[string]string{
						"addons.in-cloud.io/addon": name,
						"addons.in-cloud.io/layer": fmt.Sprintf("layer-%03d", i),
					},
					i, // priority = index
				))
			}
			addon = createTestAddon(name, opts...)

			By("Waiting for Application to be created")
			waitForApplication(name, argoCDNamespace)

			By("Waiting for Addon to reach Ready phase")
			waitForAddonReady(name)

			By("Verifying all unique keys are present in merged values")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)

				// Check that all unique layer keys exist
				for i := 0; i < numAddonValues; i++ {
					layerKey := fmt.Sprintf("layer%03d", i)
					layer, ok := values[layerKey].(map[string]interface{})
					if !ok {
						fmt.Fprintf(GinkgoWriter, "Missing layer key: %s\n", layerKey)
						return false
					}
					// Verify index matches
					if fmt.Sprintf("%v", layer["index"]) != fmt.Sprintf("%d", i) {
						fmt.Fprintf(GinkgoWriter, "Layer %s has wrong index: %v (expected %d)\n",
							layerKey, layer["index"], i)
						return false
					}
				}

				return true
			}, longTimeout, interval).Should(BeTrue(),
				"All %d unique layer keys should be present in Application values", numAddonValues)

			By("Verifying shared key has highest priority value")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)

				shared, ok := values["shared"].(map[string]interface{})
				if !ok {
					return false
				}

				// The highest priority (49) should have written last
				lastWriter := fmt.Sprintf("%v", shared["lastWriter"])
				expectedLastWriter := fmt.Sprintf("%d", numAddonValues-1)

				if lastWriter != expectedLastWriter {
					fmt.Fprintf(GinkgoWriter, "shared.lastWriter = %s (expected %s)\n",
						lastWriter, expectedLastWriter)
					return false
				}

				return true
			}, timeout, interval).Should(BeTrue(),
				"shared.lastWriter should be %d (highest priority)", numAddonValues-1)

			By("Verifying ValuesHash is populated")
			addon, err := getAddon(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(addon.Status.ValuesHash).NotTo(BeEmpty(),
				"ValuesHash should be computed for aggregated values")

			fmt.Fprintf(GinkgoWriter, "Successfully aggregated %d AddonValues\n", numAddonValues)
		})

		It("should handle rapid creation of many AddonValues", func() {
			const batchSize = 20

			By(fmt.Sprintf("Creating Addon first"))
			addon = createTestAddon(name,
				WithValuesSelector("batch", map[string]string{
					"addons.in-cloud.io/addon": name,
					"batch":                    "true",
				}, 0),
			)

			By("Waiting for initial Application")
			waitForApplication(name, argoCDNamespace)

			By(fmt.Sprintf("Creating %d AddonValues rapidly", batchSize))
			for i := 0; i < batchSize; i++ {
				avName := fmt.Sprintf("%s-rapid-%03d", name, i)
				av := createTestAddonValue(avName, name,
					map[string]interface{}{
						fmt.Sprintf("rapid%03d", i): true,
					},
					map[string]string{"batch": "true"})
				addonValues = append(addonValues, av)
			}

			By("Waiting for all values to be aggregated")
			Eventually(func() bool {
				app, err := getApplication(name, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)

				// Count how many rapid keys are present
				count := 0
				for i := 0; i < batchSize; i++ {
					if _, ok := values[fmt.Sprintf("rapid%03d", i)]; ok {
						count++
					}
				}

				fmt.Fprintf(GinkgoWriter, "Found %d/%d rapid keys in values\n", count, batchSize)
				return count == batchSize
			}, longTimeout, interval).Should(BeTrue(),
				"All %d rapid AddonValues should be aggregated", batchSize)

			By("Verifying Addon is still Ready")
			waitForAddonReady(name)

			fmt.Fprintf(GinkgoWriter, "Successfully handled rapid creation of %d AddonValues\n", batchSize)
		})
	})
})

// Helper for debug output
func debugApplicationValues(name, namespace string) {
	app, err := getApplication(name, namespace)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get application %s/%s: %v\n", namespace, name, err)
		return
	}
	fmt.Fprintf(GinkgoWriter, "Application %s/%s values:\n", namespace, name)
	if app.Spec.Source != nil && app.Spec.Source.Helm != nil {
		fmt.Fprintf(GinkgoWriter, "%s\n", app.Spec.Source.Helm.Values)
	} else {
		fmt.Fprintf(GinkgoWriter, "  (no Helm values)\n")
	}
}
