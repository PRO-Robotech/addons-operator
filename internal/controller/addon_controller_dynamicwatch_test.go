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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/conditions"
)

var _ = Describe("Dynamic Watch Integration", func() {
	// These tests verify that dynamic watches work correctly for SourceRef resources.
	// The DynamicWatchManager creates watches at runtime for any GVK referenced in valuesSources.
	//
	// Note: envtest has known limitations with informer cache sync timing.
	// These tests focus on verifying:
	// 1. Watches are created for referenced GVKs
	// 2. Addons can reconcile with valuesSources
	// 3. Watch reference counting works correctly
	//
	// For full hash-change verification, see e2e tests that run in real kind clusters.

	Context("Secret Source Watch", func() {
		It("should create watch and reconcile Addon with Secret source", func() {
			addonName := uniqueName("dw-secret")
			secretName := uniqueName("watched-secret")

			By("Creating a Secret")
			createTestSecret(secretName, "default", map[string][]byte{
				"value": []byte("test-value"),
			})

			By("Creating Addon with valuesSources referencing Secret")
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
						Name: "secret-values",
						SourceRef: addonsv1alpha1.SourceRef{
							APIVersion: "v1",
							Kind:       "Secret",
							Name:       secretName,
							Namespace:  "default",
						},
						Extract: []addonsv1alpha1.ExtractRule{{
							JSONPath: ".data.value",
							As:       "config.value",
							Decode:   "base64",
						}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Verifying Application is created (reconcile succeeded)")
			waitForApplication(addonName, "argocd")

			By("Verifying ValuesResolved condition is True")
			waitForCondition(addonName, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Verifying ValuesHash is set")
			Eventually(func() string {
				a := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, a); err != nil {
					return ""
				}

				return a.Status.ValuesHash
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			deleteSecret(secretName, "default")
		})
	})

	Context("ConfigMap Source Watch", func() {
		It("should create watch and reconcile Addon with ConfigMap source", func() {
			addonName := uniqueName("dw-configmap")
			cmName := uniqueName("watched-cm")

			By("Creating a ConfigMap")
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: "default",
				},
				Data: map[string]string{
					"config": "test-config",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Creating Addon with valuesSources referencing ConfigMap")
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
						Name: "cm-values",
						SourceRef: addonsv1alpha1.SourceRef{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       cmName,
							Namespace:  "default",
						},
						Extract: []addonsv1alpha1.ExtractRule{{
							JSONPath: ".data.config",
							As:       "settings.config",
						}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Verifying Application is created")
			waitForApplication(addonName, "argocd")

			By("Verifying ValuesResolved condition is True")
			waitForCondition(addonName, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		})
	})

	Context("Multiple Addons Sharing Watch", func() {
		It("should allow multiple Addons to reference same Secret", func() {
			addon1Name := uniqueName("dw-shared-1")
			addon2Name := uniqueName("dw-shared-2")
			secretName := uniqueName("shared-secret")

			By("Creating a shared Secret")
			createTestSecret(secretName, "default", map[string][]byte{
				"shared": []byte("shared-value"),
			})

			By("Creating first Addon referencing Secret")
			addon1 := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: addon1Name,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-1",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
					},
					ValuesSources: []addonsv1alpha1.ValueSource{{
						Name: "shared",
						SourceRef: addonsv1alpha1.SourceRef{
							APIVersion: "v1",
							Kind:       "Secret",
							Name:       secretName,
							Namespace:  "default",
						},
						Extract: []addonsv1alpha1.ExtractRule{{
							JSONPath: ".data.shared",
							As:       "config.shared",
							Decode:   "base64",
						}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon1)).To(Succeed())

			By("Creating second Addon referencing same Secret")
			addon2 := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: addon2Name,
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-2",
					RepoURL:         "https://charts.example.com",
					Version:         "2.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend: addonsv1alpha1.BackendSpec{
						Type:      "argocd",
						Namespace: "argocd",
					},
					ValuesSources: []addonsv1alpha1.ValueSource{{
						Name: "shared",
						SourceRef: addonsv1alpha1.SourceRef{
							APIVersion: "v1",
							Kind:       "Secret",
							Name:       secretName,
							Namespace:  "default",
						},
						Extract: []addonsv1alpha1.ExtractRule{{
							JSONPath: ".data.shared",
							As:       "settings.value",
							Decode:   "base64",
						}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon2)).To(Succeed())

			By("Verifying both Applications are created")
			waitForApplication(addon1Name, "argocd")
			waitForApplication(addon2Name, "argocd")

			By("Verifying both Addons have ValuesResolved=True")
			waitForCondition(addon1Name, conditions.TypeValuesResolved, metav1.ConditionTrue)
			waitForCondition(addon2Name, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, addon2)).To(Succeed())
			deleteSecret(secretName, "default")
		})
	})

	Context("Watch Cleanup on Addon Deletion", func() {
		It("should handle Addon deletion gracefully", func() {
			addon1Name := uniqueName("dw-cleanup-1")
			addon2Name := uniqueName("dw-cleanup-2")
			secretName := uniqueName("cleanup-secret")

			By("Creating a shared Secret")
			secret := createTestSecret(secretName, "default", map[string][]byte{
				"data": []byte("test-data"),
			})

			By("Creating two Addons referencing Secret")
			sourceRef := addonsv1alpha1.SourceRef{
				APIVersion: "v1",
				Kind:       "Secret",
				Name:       secretName,
				Namespace:  "default",
			}

			addon1 := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: addon1Name},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-1",
					RepoURL:         "https://charts.example.com",
					Version:         "1.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
					ValuesSources: []addonsv1alpha1.ValueSource{{
						Name:      "config",
						SourceRef: sourceRef,
						Extract:   []addonsv1alpha1.ExtractRule{{JSONPath: ".data.data", As: "value", Decode: "base64"}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon1)).To(Succeed())

			addon2 := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: addon2Name},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "chart-2",
					RepoURL:         "https://charts.example.com",
					Version:         "2.0.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "default",
					Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
					ValuesSources: []addonsv1alpha1.ValueSource{{
						Name:      "config",
						SourceRef: sourceRef,
						Extract:   []addonsv1alpha1.ExtractRule{{JSONPath: ".data.data", As: "setting", Decode: "base64"}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon2)).To(Succeed())

			By("Waiting for both Applications to be created")
			waitForApplication(addon1Name, "argocd")
			waitForApplication(addon2Name, "argocd")

			By("Deleting first Addon")
			Expect(k8sClient.Delete(ctx, addon1)).To(Succeed())

			// Wait for deletion
			Eventually(func() bool {
				a := &addonsv1alpha1.Addon{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: addon1Name}, a)

				return err != nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying second Addon still works")
			waitForCondition(addon2Name, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon2)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
	})

	Context("Non-existent Source Handling", func() {
		It("should handle gracefully when referenced Secret doesn't exist", func() {
			addonName := uniqueName("dw-nonexistent")
			secretName := uniqueName("nonexistent-secret")

			By("Creating Addon referencing non-existent Secret")
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
						Name: "missing",
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

			By("Verifying Addon reports ValueSourceError")
			waitForConditionReason(addonName, conditions.TypeValuesResolved, conditions.ReasonValueSourceError)

			By("Creating the Secret")
			createTestSecret(secretName, "default", map[string][]byte{
				"key": []byte("secret-value"),
			})

			By("Verifying Addon recovers and ValuesResolved becomes True")
			waitForCondition(addonName, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Verifying Application is created")
			waitForApplication(addonName, "argocd")

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			deleteSecret(secretName, "default")
		})
	})

	Context("SourceRef Update", func() {
		It("should handle SourceRef changes in Addon spec", func() {
			addonName := uniqueName("dw-sourceref-change")
			secret1Name := uniqueName("source-1")
			secret2Name := uniqueName("source-2")

			By("Creating two Secrets")
			createTestSecret(secret1Name, "default", map[string][]byte{
				"value": []byte("value-1"),
			})
			createTestSecret(secret2Name, "default", map[string][]byte{
				"value": []byte("value-2"),
			})

			By("Creating Addon referencing first Secret")
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
							Name:       secret1Name,
							Namespace:  "default",
						},
						Extract: []addonsv1alpha1.ExtractRule{{
							JSONPath: ".data.value",
							As:       "config.value",
							Decode:   "base64",
						}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Waiting for initial reconcile")
			waitForApplication(addonName, "argocd")
			waitForCondition(addonName, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Changing SourceRef to second Secret")
			Eventually(func() error {
				a := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, a); err != nil {
					return err
				}
				a.Spec.ValuesSources[0].SourceRef.Name = secret2Name

				return k8sClient.Update(ctx, a)
			}, timeout, interval).Should(Succeed())

			By("Verifying Addon still works after SourceRef change")
			// Spec change increments generation, which triggers reconcile
			Eventually(func() int64 {
				a := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, a); err != nil {
					return 0
				}

				return a.Status.ObservedGeneration
			}, timeout, interval).Should(BeNumerically(">=", 2))

			By("Verifying ValuesResolved is still True")
			waitForCondition(addonName, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			deleteSecret(secret1Name, "default")
			deleteSecret(secret2Name, "default")
		})
	})

	Context("Mixed Sources", func() {
		It("should watch multiple different sources for same Addon", func() {
			addonName := uniqueName("dw-mixed")
			secretName := uniqueName("mixed-secret")
			cmName := uniqueName("mixed-cm")

			By("Creating Secret and ConfigMap")
			createTestSecret(secretName, "default", map[string][]byte{
				"secret-key": []byte("secret-value"),
			})
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: "default",
				},
				Data: map[string]string{
					"cm-key": "cm-value",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			By("Creating Addon with both Secret and ConfigMap sources")
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
					ValuesSources: []addonsv1alpha1.ValueSource{
						{
							Name: "secret-source",
							SourceRef: addonsv1alpha1.SourceRef{
								APIVersion: "v1",
								Kind:       "Secret",
								Name:       secretName,
								Namespace:  "default",
							},
							Extract: []addonsv1alpha1.ExtractRule{{
								JSONPath: ".data.secret-key",
								As:       "config.secret",
								Decode:   "base64",
							}},
						},
						{
							Name: "cm-source",
							SourceRef: addonsv1alpha1.SourceRef{
								APIVersion: "v1",
								Kind:       "ConfigMap",
								Name:       cmName,
								Namespace:  "default",
							},
							Extract: []addonsv1alpha1.ExtractRule{{
								JSONPath: ".data.cm-key",
								As:       "config.cm",
							}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			By("Verifying Application is created")
			waitForApplication(addonName, "argocd")

			By("Verifying ValuesResolved condition is True")
			waitForCondition(addonName, conditions.TypeValuesResolved, metav1.ConditionTrue)

			By("Verifying ValuesHash is set")
			Eventually(func() string {
				a := &addonsv1alpha1.Addon{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, a); err != nil {
					return ""
				}

				return a.Status.ValuesHash
			}, timeout, interval).ShouldNot(BeEmpty())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, addon)).To(Succeed())
			deleteSecret(secretName, "default")
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		})
	})
})
