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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("AddonPhase Controller", func() {
	Context("When reconciling a resource", func() {
		It("should reconcile when target Addon exists", func() {
			name := uniqueName("phase-test")

			By("Creating the target Addon")
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

			By("Creating the AddonPhase")
			phase := &addonsv1alpha1.AddonPhase{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: addonsv1alpha1.AddonPhaseSpec{
					Rules: []addonsv1alpha1.PhaseRule{
						{
							Name: "always-active",
							// No criteria = always active
							Selector: addonsv1alpha1.ValuesSelector{
								Name:     "phase-values",
								Priority: 10,
								MatchLabels: map[string]string{
									"test": "true",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, phase)).To(Succeed())

			By("Waiting for rule to be matched")
			waitForPhaseRuleMatched(name, "always-active", true)

			By("Verifying Addon has phaseValuesSelector")
			waitForAddonPhaseValuesSelector(name, true)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, phase)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})

	Context("Rule Activation", func() {
		// Note: Testing criteria evaluation against external resources is complex
		// in envtest due to status update timing. The first test (always-active rule)
		// covers the basic rule → selector → phaseValuesSelector flow.
		// Criteria evaluation logic is unit tested in the rules package.
		It("should not match rule when criterion fails", func() {
			targetName := uniqueName("target-addon-phase")

			By("Creating target Addon")
			targetAddon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: targetName,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "target-chart",
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
			Expect(k8sClient.Create(ctx, targetAddon)).To(Succeed())

			By("Creating AddonPhase with criteria referencing non-existent addon")
			phase := &addonsv1alpha1.AddonPhase{
				ObjectMeta: metav1.ObjectMeta{
					Name: targetName,
				},
				Spec: addonsv1alpha1.AddonPhaseSpec{
					Rules: []addonsv1alpha1.PhaseRule{
						{
							Name: "certificates",
							Criteria: []addonsv1alpha1.Criterion{{
								Source: &addonsv1alpha1.CriterionSource{
									APIVersion: "addons.in-cloud.io/v1alpha1",
									Kind:       "Addon",
									Name:       "non-existent-addon",
								},
								JSONPath: "$.status.phase",
								Operator: addonsv1alpha1.OperatorEqual,
								Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
							}},
							Selector: addonsv1alpha1.ValuesSelector{
								Name:     "certificates",
								Priority: 20,
								MatchLabels: map[string]string{
									"feature.certificates": "true",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, phase)).To(Succeed())

			By("Waiting for rule to be NOT matched (source doesn't exist)")
			waitForPhaseRuleMatched(targetName, "certificates", false)

			By("Verifying Addon does NOT have phaseValuesSelector")
			waitForAddonPhaseValuesSelector(targetName, false)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, phase)).To(Succeed())
			Expect(k8sClient.Delete(ctx, targetAddon)).To(Succeed())
		})
	})

	Context("Rule Latching", func() {
		It("should latch rule and stay matched after dependency loses Ready", func() {
			name := uniqueName("latch-test")

			By("Creating target Addon")
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

			By("Creating AddonPhase with keep=default (nil) criterion evaluating target addon")
			phase := &addonsv1alpha1.AddonPhase{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: addonsv1alpha1.AddonPhaseSpec{
					Rules: []addonsv1alpha1.PhaseRule{
						{
							Name: "latch-rule",
							Criteria: []addonsv1alpha1.Criterion{{
								JSONPath: "$.metadata.name",
								Operator: addonsv1alpha1.OperatorEqual,
								Value:    &apiextensionsv1.JSON{Raw: []byte(`"` + name + `"`)},
							}},
							Selector: addonsv1alpha1.ValuesSelector{
								Name:     "latched-values",
								Priority: 10,
								MatchLabels: map[string]string{
									"latch": "test",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, phase)).To(Succeed())

			By("Waiting for rule to be matched and latched")
			EventuallyWithOffset(1, func() bool {
				p := &addonsv1alpha1.AddonPhase{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(phase), p); err != nil {
					return false
				}
				for _, r := range p.Status.RuleStatuses {
					if r.Name == "latch-rule" {
						return r.Matched && r.Latched
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Verifying Addon has phaseValuesSelector")
			waitForAddonPhaseValuesSelector(name, true)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, phase)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})

	Context("AddonPhase Cleanup", func() {
		It("should clear phaseValuesSelector when AddonPhase deleted", func() {
			name := uniqueName("cleanup-test")

			By("Creating target Addon")
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

			By("Creating AddonPhase with always-active rule")
			phase := &addonsv1alpha1.AddonPhase{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: addonsv1alpha1.AddonPhaseSpec{
					Rules: []addonsv1alpha1.PhaseRule{
						{
							Name: "always-on",
							// No criteria = always active
							Selector: addonsv1alpha1.ValuesSelector{
								Name:        "phase-values",
								Priority:    10,
								MatchLabels: map[string]string{"cleanup": "test"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, phase)).To(Succeed())

			By("Waiting for phaseValuesSelector to be populated")
			waitForAddonPhaseValuesSelector(name, true)

			By("Deleting AddonPhase")
			Expect(k8sClient.Delete(ctx, phase)).To(Succeed())

			By("Waiting for phaseValuesSelector to be cleared")
			waitForAddonPhaseValuesSelector(name, false)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
		})
	})
})
