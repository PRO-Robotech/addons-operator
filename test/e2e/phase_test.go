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

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("AddonPhase Activation", Ordered, func() {
	// Initialize k8s client before all tests
	BeforeAll(func() {
		By("initializing k8s client")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred(), "Failed to initialize k8s client")
		}
	})

	Context("Scenario 2: AddonPhase Activation (PRD 2.2)", func() {
		var depName, targetName string
		var depAddon, targetAddon *addonsv1alpha1.Addon
		var targetPhase *addonsv1alpha1.AddonPhase
		var certValues *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			depName = uniqueName("dep-addon")
			targetName = uniqueName("target-addon")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			// Clean up in reverse order of creation
			if targetPhase != nil {
				cleanupResource(targetPhase)
			}
			if certValues != nil {
				cleanupResource(certValues)
			}
			if targetAddon != nil {
				cleanupResource(targetAddon)
			}
			if depAddon != nil {
				cleanupResource(depAddon)
			}
		})

		It("should activate rule when dependency becomes Ready", func() {
			By("Creating dependency Addon")
			depAddon = createTestAddon(depName)

			By("Creating target Addon")
			targetAddon = createTestAddon(targetName)

			By("Creating AddonValue for certificates feature")
			certValues = createTestAddonValue(targetName+"-certs", targetName,
				map[string]interface{}{
					"tls": map[string]interface{}{
						"enabled": true,
						"mode":    "strict",
					},
				},
				map[string]string{"feature.certificates": "true"})

			By("Creating AddonPhase with dependency on dep-addon")
			// Build criterion: dep-addon Ready condition status == "True"
			criterion := NewCriterion().
				WithSource("addons.in-cloud.io/v1alpha1", "Addon", depName, "").
				WithJSONPath("/status/conditions[?(@.type=='Ready')]/status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("certificates", []addonsv1alpha1.Criterion{criterion},
					addonsv1alpha1.ValuesSelector{
						Name:     "certificates",
						Priority: 20,
						MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": targetName,
							"feature.certificates":     "true",
						},
					}),
			)

			By("Waiting for dependency to become Ready")
			waitForAddonReady(depName)

			By("Verifying rule becomes matched")
			waitForPhaseRuleMatched(targetName, "certificates", true)

			By("Verifying phaseValuesSelector is populated in target Addon")
			waitForPhaseValuesSelector(targetName, 1)

			By("Verifying the injected selector has correct priority")
			Eventually(func() bool {
				addon, err := getAddon(targetName)
				if err != nil {
					return false
				}
				for _, sel := range addon.Status.PhaseValuesSelector {
					if sel.Name == "certificates" && sel.Priority == 20 {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"PhaseValuesSelector should contain certificates selector with priority 20")
		})
	})

	Context("Multiple Rules", func() {
		var depName1, depName2, targetName string
		var depAddon1, depAddon2, targetAddon *addonsv1alpha1.Addon
		var targetPhase *addonsv1alpha1.AddonPhase
		var feature1Values, feature2Values *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			depName1 = uniqueName("dep1")
			depName2 = uniqueName("dep2")
			targetName = uniqueName("multi-rule")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if targetPhase != nil {
				cleanupResource(targetPhase)
			}
			if feature1Values != nil {
				cleanupResource(feature1Values)
			}
			if feature2Values != nil {
				cleanupResource(feature2Values)
			}
			if targetAddon != nil {
				cleanupResource(targetAddon)
			}
			if depAddon1 != nil {
				cleanupResource(depAddon1)
			}
			if depAddon2 != nil {
				cleanupResource(depAddon2)
			}
		})

		It("should evaluate multiple rules independently", func() {
			By("Creating two dependency Addons")
			depAddon1 = createTestAddon(depName1)
			depAddon2 = createTestAddon(depName2)

			By("Creating target Addon")
			targetAddon = createTestAddon(targetName)

			By("Creating AddonValues for two features")
			feature1Values = createTestAddonValue(targetName+"-feature1", targetName,
				map[string]interface{}{"feature1": map[string]interface{}{"enabled": true}},
				map[string]string{"feature": "one"})

			feature2Values = createTestAddonValue(targetName+"-feature2", targetName,
				map[string]interface{}{"feature2": map[string]interface{}{"enabled": true}},
				map[string]string{"feature": "two"})

			By("Creating AddonPhase with two independent rules")
			criterion1 := NewCriterion().
				WithSource("addons.in-cloud.io/v1alpha1", "Addon", depName1, "").
				WithJSONPath("/status/conditions[?(@.type=='Ready')]/status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			criterion2 := NewCriterion().
				WithSource("addons.in-cloud.io/v1alpha1", "Addon", depName2, "").
				WithJSONPath("/status/conditions[?(@.type=='Ready')]/status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("feature-one", []addonsv1alpha1.Criterion{criterion1},
					addonsv1alpha1.ValuesSelector{
						Name:        "feature-one",
						Priority:    10,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": targetName, "feature": "one"},
					}),
				WithPhaseRule("feature-two", []addonsv1alpha1.Criterion{criterion2},
					addonsv1alpha1.ValuesSelector{
						Name:        "feature-two",
						Priority:    15,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": targetName, "feature": "two"},
					}),
			)

			By("Waiting for first dependency to become Ready")
			waitForAddonReady(depName1)

			By("Verifying first rule matched, second not yet")
			waitForPhaseRuleMatched(targetName, "feature-one", true)
			// Note: feature-two may or may not be matched yet depending on dep2 status

			By("Waiting for second dependency to become Ready")
			waitForAddonReady(depName2)

			By("Verifying both rules are matched")
			waitForPhaseRuleMatched(targetName, "feature-one", true)
			waitForPhaseRuleMatched(targetName, "feature-two", true)

			By("Verifying both selectors are in phaseValuesSelector")
			waitForPhaseValuesSelector(targetName, 2)
		})
	})

	Context("Criteria Operators", func() {
		var targetName, secretName string
		var targetAddon *addonsv1alpha1.Addon
		var targetPhase *addonsv1alpha1.AddonPhase
		var featureValues *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			targetName = uniqueName("operators")
			secretName = targetName + "-config"
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if targetPhase != nil {
				cleanupResource(targetPhase)
			}
			if featureValues != nil {
				cleanupResource(featureValues)
			}
			if targetAddon != nil {
				cleanupResource(targetAddon)
			}
			deleteTestSecret(secretName, testNamespace)
		})

		It("should support Exists operator", func() {
			By("Creating target Addon")
			targetAddon = createTestAddon(targetName)

			By("Creating AddonValue for config feature")
			featureValues = createTestAddonValue(targetName+"-config", targetName,
				map[string]interface{}{"config": map[string]interface{}{"fromSecret": true}},
				map[string]string{"feature": "config"})

			By("Creating AddonPhase with Exists criterion on Secret")
			// Check if Secret exists (data field exists)
			criterion := NewCriterion().
				WithSource("v1", "Secret", secretName, testNamespace).
				WithJSONPath("/data").
				WithOperator(addonsv1alpha1.OperatorExists).
				Build()

			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("config-exists", []addonsv1alpha1.Criterion{criterion},
					addonsv1alpha1.ValuesSelector{
						Name:        "config",
						Priority:    30,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": targetName, "feature": "config"},
					}),
			)

			By("Verifying rule is NOT matched (Secret doesn't exist)")
			// Give controller time to evaluate
			Consistently(func() bool {
				phase, err := getAddonPhase(targetName)
				if err != nil {
					return false
				}
				for _, rs := range phase.Status.RuleStatuses {
					if rs.Name == "config-exists" && rs.Matched {
						return true // Should NOT be matched
					}
				}
				return false
			}, "10s", interval).Should(BeFalse(),
				"Rule should not be matched when Secret doesn't exist")

			By("Creating the Secret")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"key": []byte("value"),
			})

			By("Verifying rule becomes matched after Secret creation")
			waitForPhaseRuleMatched(targetName, "config-exists", true)
		})

		It("should support NotEqual operator", func() {
			By("Creating target Addon")
			targetAddon = createTestAddon(targetName)

			By("Creating AddonValue")
			featureValues = createTestAddonValue(targetName+"-notequal", targetName,
				map[string]interface{}{"notequal": true},
				map[string]string{"feature": "notequal"})

			By("Creating AddonPhase with NotEqual criterion")
			// Check that Addon Ready condition status is NOT "False" (i.e., not failed)
			criterion := NewCriterion().
				WithSource("addons.in-cloud.io/v1alpha1", "Addon", targetName, "").
				WithJSONPath("/status/conditions[?(@.type=='Ready')]/status").
				WithOperator(addonsv1alpha1.OperatorNotEqual).
				WithValue("False").
				Build()

			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("not-failed", []addonsv1alpha1.Criterion{criterion},
					addonsv1alpha1.ValuesSelector{
						Name:        "not-failed",
						Priority:    5,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": targetName, "feature": "notequal"},
					}),
			)

			By("Waiting for Addon to have conditions")
			Eventually(func() bool {
				addon, err := getAddon(targetName)
				if err != nil {
					return false
				}
				return len(addon.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue())

			By("Verifying rule is matched (phase is not Failed)")
			waitForPhaseRuleMatched(targetName, "not-failed", true)
		})
	})

	Context("Unconditional Rule", func() {
		var targetName string
		var targetAddon *addonsv1alpha1.Addon
		var targetPhase *addonsv1alpha1.AddonPhase
		var alwaysValues *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			targetName = uniqueName("unconditional")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if targetPhase != nil {
				cleanupResource(targetPhase)
			}
			if alwaysValues != nil {
				cleanupResource(alwaysValues)
			}
			if targetAddon != nil {
				cleanupResource(targetAddon)
			}
		})

		It("should always match rule with empty criteria", func() {
			By("Creating target Addon")
			targetAddon = createTestAddon(targetName)

			By("Creating AddonValue")
			alwaysValues = createTestAddonValue(targetName+"-always", targetName,
				map[string]interface{}{"always": map[string]interface{}{"applied": true}},
				map[string]string{"feature": "always"})

			By("Creating AddonPhase with empty criteria (always matches)")
			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("always-on", []addonsv1alpha1.Criterion{}, // Empty criteria = always match
					addonsv1alpha1.ValuesSelector{
						Name:        "always-on",
						Priority:    99,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": targetName, "feature": "always"},
					}),
			)

			By("Verifying rule is immediately matched")
			waitForPhaseRuleMatched(targetName, "always-on", true)

			By("Verifying phaseValuesSelector is populated")
			waitForPhaseValuesSelector(targetName, 1)
		})
	})

	Context("Rule Deactivation", func() {
		var depName, targetName string
		var depAddon, targetAddon *addonsv1alpha1.Addon
		var targetPhase *addonsv1alpha1.AddonPhase
		var featureValues *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			depName = uniqueName("deact-dep")
			targetName = uniqueName("deact-target")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if targetPhase != nil {
				cleanupResource(targetPhase)
			}
			if featureValues != nil {
				cleanupResource(featureValues)
			}
			if targetAddon != nil {
				cleanupResource(targetAddon)
			}
			if depAddon != nil {
				cleanupResource(depAddon)
			}
		})

		It("should deactivate rule when dependency is deleted", func() {
			By("Creating dependency Addon")
			depAddon = createTestAddon(depName)

			By("Creating target Addon")
			targetAddon = createTestAddon(targetName)

			By("Creating AddonValue for feature")
			featureValues = createTestAddonValue(targetName+"-feature", targetName,
				map[string]interface{}{
					"feature": map[string]interface{}{
						"enabled": true,
						"mode":    "advanced",
					},
				},
				map[string]string{"feature": "deactivation-test"})

			By("Creating AddonPhase with dependency on dep-addon")
			criterion := NewCriterion().
				WithSource("addons.in-cloud.io/v1alpha1", "Addon", depName, "").
				WithJSONPath("/status/conditions[?(@.type=='Ready')]/status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("dep-feature", []addonsv1alpha1.Criterion{criterion},
					addonsv1alpha1.ValuesSelector{
						Name:     "dep-feature",
						Priority: 25,
						MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": targetName,
							"feature":                  "deactivation-test",
						},
					}),
			)

			By("Waiting for dependency to become Ready")
			waitForAddonReady(depName)

			By("Verifying rule is matched")
			waitForPhaseRuleMatched(targetName, "dep-feature", true)

			By("Verifying phaseValuesSelector is populated")
			waitForPhaseValuesSelector(targetName, 1)

			By("Recording current phaseValuesSelector state")
			addon, err := getAddon(targetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(addon.Status.PhaseValuesSelector).To(HaveLen(1))

			By("Deleting the dependency Addon")
			depAddonCopy := depAddon.DeepCopy()
			Expect(k8sClient.Delete(ctx, depAddon)).To(Succeed())
			depAddon = nil // Prevent double cleanup

			By("Waiting for dependency to be fully deleted")
			waitForDeletion(depAddonCopy)

			By("Verifying rule becomes unmatched after dependency deletion")
			waitForPhaseRuleMatched(targetName, "dep-feature", false)

			By("Verifying phaseValuesSelector is cleared")
			Eventually(func() int {
				addon, err := getAddon(targetName)
				if err != nil {
					return -1
				}
				return len(addon.Status.PhaseValuesSelector)
			}, timeout, interval).Should(Equal(0),
				"phaseValuesSelector should be empty after dependency deletion")
		})

		It("should deactivate rule when external resource is deleted", func() {
			secretName := targetName + "-config"

			By("Creating target Addon")
			targetAddon = createTestAddon(targetName)

			By("Creating Secret that the rule depends on")
			createTestSecret(secretName, testNamespace, map[string][]byte{
				"enabled": []byte("true"),
			})

			By("Creating AddonValue for feature")
			featureValues = createTestAddonValue(targetName+"-feature", targetName,
				map[string]interface{}{
					"config": map[string]interface{}{"fromSecret": true},
				},
				map[string]string{"feature": "secret-dep"})

			By("Creating AddonPhase with Exists criterion on Secret")
			criterion := NewCriterion().
				WithSource("v1", "Secret", secretName, testNamespace).
				WithJSONPath("/data").
				WithOperator(addonsv1alpha1.OperatorExists).
				Build()

			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("secret-exists", []addonsv1alpha1.Criterion{criterion},
					addonsv1alpha1.ValuesSelector{
						Name:        "secret-config",
						Priority:    30,
						MatchLabels: map[string]string{"addons.in-cloud.io/addon": targetName, "feature": "secret-dep"},
					}),
			)

			By("Verifying rule is matched (Secret exists)")
			waitForPhaseRuleMatched(targetName, "secret-exists", true)

			By("Verifying phaseValuesSelector is populated")
			waitForPhaseValuesSelector(targetName, 1)

			By("Deleting the Secret")
			deleteTestSecret(secretName, testNamespace)

			By("Verifying rule becomes unmatched after Secret deletion")
			waitForPhaseRuleMatched(targetName, "secret-exists", false)

			By("Verifying phaseValuesSelector is cleared")
			Eventually(func() int {
				addon, err := getAddon(targetName)
				if err != nil {
					return -1
				}
				return len(addon.Status.PhaseValuesSelector)
			}, timeout, interval).Should(Equal(0),
				"phaseValuesSelector should be empty after Secret deletion")
		})
	})
})

var _ = Describe("AddonPhase Values Integration", Ordered, func() {
	BeforeAll(func() {
		By("ensuring k8s client is initialized")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Phase Values in Application", func() {
		var depName, targetName string
		var depAddon, targetAddon *addonsv1alpha1.Addon
		var targetPhase *addonsv1alpha1.AddonPhase
		var baseValues, phaseValues *addonsv1alpha1.AddonValue

		BeforeEach(func() {
			depName = uniqueName("phase-dep")
			targetName = uniqueName("phase-target")
		})

		AfterEach(func() {
			By("cleaning up test resources")
			if targetPhase != nil {
				cleanupResource(targetPhase)
			}
			if phaseValues != nil {
				cleanupResource(phaseValues)
			}
			if baseValues != nil {
				cleanupResource(baseValues)
			}
			if targetAddon != nil {
				cleanupResource(targetAddon)
			}
			if depAddon != nil {
				cleanupResource(depAddon)
			}
		})

		It("should merge phase values into Application", func() {
			By("Creating dependency Addon")
			depAddon = createTestAddon(depName)

			By("Creating base AddonValue")
			baseValues = createTestAddonValue(targetName+"-base", targetName,
				map[string]interface{}{
					"base":    map[string]interface{}{"enabled": true},
					"feature": map[string]interface{}{"mode": "basic"},
				},
				map[string]string{"addons.in-cloud.io/layer": "base"})

			By("Creating phase-activated AddonValue")
			phaseValues = createTestAddonValue(targetName+"-phase", targetName,
				map[string]interface{}{
					"feature": map[string]interface{}{"mode": "advanced", "extra": true},
				},
				map[string]string{"addons.in-cloud.io/layer": "phase"})

			By("Creating target Addon with base selector")
			targetAddon = createTestAddon(targetName,
				WithValuesSelector("base", map[string]string{
					"addons.in-cloud.io/addon": targetName,
					"addons.in-cloud.io/layer": "base",
				}, 0),
			)

			By("Waiting for Application with base values")
			waitForApplication(targetName, argoCDNamespace)

			By("Creating AddonPhase to activate phase values when dep is Ready")
			criterion := NewCriterion().
				WithSource("addons.in-cloud.io/v1alpha1", "Addon", depName, "").
				WithJSONPath("/status/conditions[?(@.type=='Ready')]/status").
				WithOperator(addonsv1alpha1.OperatorEqual).
				WithValue("True").
				Build()

			targetPhase = createTestAddonPhase(targetName,
				WithPhaseRule("advanced-mode", []addonsv1alpha1.Criterion{criterion},
					addonsv1alpha1.ValuesSelector{
						Name:     "phase-values",
						Priority: 50, // Higher than base (0)
						MatchLabels: map[string]string{
							"addons.in-cloud.io/addon": targetName,
							"addons.in-cloud.io/layer": "phase",
						},
					}),
			)

			By("Waiting for dependency to become Ready")
			waitForAddonReady(depName)

			By("Waiting for phase rule to match")
			waitForPhaseRuleMatched(targetName, "advanced-mode", true)

			By("Verifying Application values contain merged phase values")
			Eventually(func() bool {
				app, err := getApplication(targetName, argoCDNamespace)
				if err != nil {
					return false
				}
				values := getApplicationValues(app)

				// Check base value is present
				base, hasBase := values["base"].(map[string]interface{})
				if !hasBase || base["enabled"] != true {
					return false
				}

				// Check feature was overridden by phase values
				feature, hasFeature := values["feature"].(map[string]interface{})
				if !hasFeature {
					return false
				}

				// mode should be "advanced" (from phase, priority 50)
				// extra should be true (from phase)
				return feature["mode"] == "advanced" && feature["extra"] == true
			}, longTimeout, interval).Should(BeTrue(),
				"Application should have merged base + phase values")
		})
	})
})

// Helper for debug output
func debugPhaseStatus(name string) {
	phase, err := getAddonPhase(name)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get AddonPhase %s: %v\n", name, err)
		return
	}
	fmt.Fprintf(GinkgoWriter, "AddonPhase %s status:\n", name)
	fmt.Fprintf(GinkgoWriter, "  ObservedGeneration: %d\n", phase.Status.ObservedGeneration)
	fmt.Fprintf(GinkgoWriter, "  RuleStatuses:\n")
	for _, rs := range phase.Status.RuleStatuses {
		fmt.Fprintf(GinkgoWriter, "    - %s: matched=%v (%s)\n", rs.Name, rs.Matched, rs.Message)
	}
}
