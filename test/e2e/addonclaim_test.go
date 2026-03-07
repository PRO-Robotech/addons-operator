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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// addonTemplateForE2E is the Go template used by AddonClaim e2e tests.
// It renders an Addon resource using variables from the claim.
const addonTemplateForE2E = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" }}
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  valuesSelectors:
  - name: claim-values
    priority: 0
    matchLabels:
      addons.in-cloud.io/addon: {{ index .Values.spec.variables "name" }}
      addons.in-cloud.io/values: claim
`

var _ = Describe("AddonClaim", Ordered, func() {
	var (
		templateName string
		secretName   string
	)

	BeforeAll(func() {
		By("initializing k8s client")
		if k8sClient == nil {
			err := initK8sClient()
			Expect(err).NotTo(HaveOccurred(), "Failed to initialize k8s client")
		}

		By("creating shared AddonTemplate for AddonClaim tests")
		templateName = uniqueName("claim-tmpl")
		createTestAddonTemplate(templateName, addonTemplateForE2E)

		By("creating shared kubeconfig Secret for AddonClaim tests")
		secretName = uniqueName("claim-kubeconfig")
		createKubeconfigSecret(secretName, testNamespace)
	})

	AfterAll(func() {
		By("cleaning up shared AddonTemplate")
		tmpl := &addonsv1alpha1.AddonTemplate{}
		tmpl.Name = templateName
		cleanupResource(tmpl)

		By("cleaning up shared kubeconfig Secret")
		deleteTestSecret(secretName, testNamespace)
	})

	Context("Basic AddonClaim Deployment", func() {
		var (
			claimName string
			addonName string
		)

		BeforeEach(func() {
			claimName = uniqueName("claim-basic")
			addonName = claimName // template uses variables.name = claimName
		})

		AfterEach(func() {
			By("cleaning up AddonClaim")
			claim := &addonsv1alpha1.AddonClaim{}
			claim.Name = claimName
			claim.Namespace = testNamespace
			cleanupResource(claim)

			By("cleaning up remote Addon")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = addonName
			cleanupResource(addon)

			By("cleaning up remote AddonValue")
			av := &addonsv1alpha1.AddonValue{}
			av.Name = addonName + "-claim-values"
			cleanupResource(av)
		})

		It("should create Addon from AddonClaim with variables", func() {
			By("Creating AddonClaim with variables")
			createTestAddonClaim(claimName, testNamespace, templateName, secretName,
				WithClaimAddon(claimName),
				WithClaimVariables(map[string]string{
					"name":    claimName,
					"version": "6.5.0",
				}),
			)

			By("Waiting for TemplateRendered=True")
			waitForClaimCondition(claimName, testNamespace, "TemplateRendered", metav1.ConditionTrue)

			By("Waiting for RemoteConnected=True")
			waitForClaimCondition(claimName, testNamespace, "RemoteConnected", metav1.ConditionTrue)

			By("Waiting for AddonSynced=True")
			waitForClaimCondition(claimName, testNamespace, "AddonSynced", metav1.ConditionTrue)

			By("Verifying Addon is created with correct spec")
			Eventually(func() error {
				addon, err := getAddon(addonName)
				if err != nil {
					return err
				}
				if addon.Spec.Chart != "podinfo" {
					return fmt.Errorf("expected chart=podinfo, got %s", addon.Spec.Chart)
				}
				if addon.Spec.Version != "6.5.0" {
					return fmt.Errorf("expected version=6.5.0, got %s", addon.Spec.Version)
				}
				return nil
			}, timeout, interval).Should(Succeed(), "Addon should be created with correct spec")

			By("Waiting for Application to be created in argocd namespace")
			waitForApplication(addonName, argoCDNamespace)
		})
	})

	Context("AddonClaim with Values", func() {
		var (
			claimName string
			addonName string
		)

		BeforeEach(func() {
			claimName = uniqueName("claim-vals")
			addonName = claimName
		})

		AfterEach(func() {
			By("cleaning up AddonClaim")
			claim := &addonsv1alpha1.AddonClaim{}
			claim.Name = claimName
			claim.Namespace = testNamespace
			cleanupResource(claim)

			By("cleaning up remote Addon")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = addonName
			cleanupResource(addon)

			By("cleaning up remote AddonValue")
			av := &addonsv1alpha1.AddonValue{}
			av.Name = addonName + "-claim-values"
			cleanupResource(av)
		})

		It("should create AddonValue when valuesString is provided", func() {
			By("Creating AddonClaim with valuesString")
			createTestAddonClaim(claimName, testNamespace, templateName, secretName,
				WithClaimAddon(claimName),
				WithClaimVariables(map[string]string{
					"name":    claimName,
					"version": "6.5.0",
				}),
				WithClaimValuesString("replicaCount: 2\nui:\n  message: hello-e2e\n"),
			)

			By("Waiting for AddonSynced=True")
			waitForClaimCondition(claimName, testNamespace, "AddonSynced", metav1.ConditionTrue)

			By("Verifying AddonValue is created with correct labels and values")
			addonValueName := addonName + "-claim-values"
			Eventually(func() error {
				av := &addonsv1alpha1.AddonValue{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonValueName}, av); err != nil {
					return err
				}
				if av.Labels["addons.in-cloud.io/addon"] != addonName {
					return fmt.Errorf("expected addon label=%s, got %s", addonName, av.Labels["addons.in-cloud.io/addon"])
				}
				if av.Labels["addons.in-cloud.io/values"] != "claim" {
					return fmt.Errorf("expected values label=claim, got %s", av.Labels["addons.in-cloud.io/values"])
				}
				if av.Spec.Values == "" {
					return fmt.Errorf("expected values to be populated")
				}
				return nil
			}, timeout, interval).Should(Succeed(), "AddonValue should be created with correct content")
		})
	})

	Context("AddonClaim Update", func() {
		var (
			claimName string
			addonName string
		)

		BeforeEach(func() {
			claimName = uniqueName("claim-upd")
			addonName = claimName
		})

		AfterEach(func() {
			By("cleaning up AddonClaim")
			claim := &addonsv1alpha1.AddonClaim{}
			claim.Name = claimName
			claim.Namespace = testNamespace
			cleanupResource(claim)

			By("cleaning up remote Addon")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = addonName
			cleanupResource(addon)

			By("cleaning up remote AddonValue")
			av := &addonsv1alpha1.AddonValue{}
			av.Name = addonName + "-claim-values"
			cleanupResource(av)
		})

		It("should update Addon when variables change", func() {
			By("Creating AddonClaim with version=6.5.0")
			createTestAddonClaim(claimName, testNamespace, templateName, secretName,
				WithClaimAddon(claimName),
				WithClaimVariables(map[string]string{
					"name":    claimName,
					"version": "6.5.0",
				}),
			)

			By("Waiting for Addon with version 6.5.0")
			Eventually(func() string {
				addon, err := getAddon(addonName)
				if err != nil {
					return ""
				}
				return addon.Spec.Version
			}, timeout, interval).Should(Equal("6.5.0"), "Addon should have version 6.5.0")

			By("Updating AddonClaim variables to version=6.6.0")
			Eventually(func() error {
				fresh, err := getAddonClaim(claimName, testNamespace)
				if err != nil {
					return err
				}
				fresh.Spec.Variables = jsonFromMap(map[string]string{
					"name":    claimName,
					"version": "6.6.0",
				})
				return k8sClient.Update(ctx, fresh)
			}, timeout, interval).Should(Succeed(), "Should update AddonClaim variables")

			By("Waiting for Addon version to be updated to 6.6.0")
			Eventually(func() string {
				addon, err := getAddon(addonName)
				if err != nil {
					return ""
				}
				return addon.Spec.Version
			}, timeout, interval).Should(Equal("6.6.0"), "Addon should be updated to version 6.6.0")
		})
	})

	Context("AddonClaim Deletion", func() {
		var (
			claimName string
			addonName string
		)

		BeforeEach(func() {
			claimName = uniqueName("claim-del")
			addonName = claimName
		})

		It("should delete remote Addon when AddonClaim is deleted", func() {
			By("Creating AddonClaim")
			claim := createTestAddonClaim(claimName, testNamespace, templateName, secretName,
				WithClaimAddon(claimName),
				WithClaimVariables(map[string]string{
					"name":    claimName,
					"version": "6.5.0",
				}),
			)

			By("Waiting for Addon to be created")
			Eventually(func() error {
				_, err := getAddon(addonName)
				return err
			}, timeout, interval).Should(Succeed(), "Addon should be created")

			By("Deleting AddonClaim")
			Expect(k8sClient.Delete(ctx, claim)).To(Succeed())

			By("Waiting for remote Addon to be deleted")
			Eventually(func() bool {
				_, err := getAddon(addonName)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "Addon should be deleted when AddonClaim is deleted")

			By("Waiting for AddonClaim to be fully deleted (finalizer removed)")
			Eventually(func() bool {
				_, err := getAddonClaim(claimName, testNamespace)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "AddonClaim should be fully deleted")
		})
	})

	Context("External Status", func() {
		var (
			claimName string
			addonName string
		)

		BeforeEach(func() {
			claimName = uniqueName("claim-ext")
			addonName = claimName
		})

		AfterEach(func() {
			By("cleaning up AddonClaim")
			claim := &addonsv1alpha1.AddonClaim{}
			claim.Name = claimName
			claim.Namespace = testNamespace
			cleanupResource(claim)

			By("cleaning up remote Addon")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = addonName
			cleanupResource(addon)

			By("cleaning up remote AddonValue")
			av := &addonsv1alpha1.AddonValue{}
			av.Name = addonName + "-claim-values"
			cleanupResource(av)
		})

		It("should populate CAPI status fields when annotation is present", func() {
			By("Creating AddonClaim with external-status/type annotation")
			createTestAddonClaim(claimName, testNamespace, templateName, secretName,
				WithClaimAddon(claimName),
				WithClaimVariables(map[string]string{
					"name":    claimName,
					"version": "6.5.0",
				}),
				WithClaimVersion("6.5.0"),
				WithClaimAnnotations(map[string]string{
					"external-status/type": "controlplane",
				}),
			)

			By("Waiting for AddonSynced=True")
			waitForClaimCondition(claimName, testNamespace, "AddonSynced", metav1.ConditionTrue)

			By("Verifying CAPI flat fields are populated")
			Eventually(func() bool {
				claim, err := getAddonClaim(claimName, testNamespace)
				if err != nil {
					return false
				}
				return claim.Status.ExternalManagedControlPlane != nil
			}, timeout, interval).Should(BeTrue(), "ExternalManagedControlPlane should be populated")

			By("Verifying externalManagedControlPlane is true")
			claim, err := getAddonClaim(claimName, testNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(claim.Status.ExternalManagedControlPlane).To(HaveValue(BeTrue()))

			By("Verifying version from variables")
			Expect(claim.Status.Version).To(Equal("6.5.0"))

			By("Verifying initialized fields are set")
			Expect(claim.Status.Initialized).NotTo(BeNil())
			Expect(claim.Status.Initialization).NotTo(BeNil())
			Expect(claim.Status.Initialization.ControlPlaneInitialized).NotTo(BeNil())
		})
	})

	Context("AddonClaim with Vars shortcut and custom valueLabels", func() {
		var (
			claimName    string
			addonName    string
			varsTemplate string
		)

		BeforeEach(func() {
			claimName = uniqueName("claim-vars")
			addonName = claimName

			varsTemplate = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Vars.name }}
spec:
  chart: podinfo
  repoURL: https://stefanprodan.github.io/podinfo
  version: "{{ .Vars.version }}"
  targetCluster: in-cluster
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
  valuesSelectors:
  - name: custom-values
    priority: 0
    matchLabels:
      addons.in-cloud.io/addon: {{ .Vars.name }}
      addons.in-cloud.io/values: custom
`
		})

		AfterEach(func() {
			By("cleaning up AddonClaim")
			claim := &addonsv1alpha1.AddonClaim{}
			claim.Name = claimName
			claim.Namespace = testNamespace
			cleanupResource(claim)

			By("cleaning up custom AddonTemplate")
			tmpl := &addonsv1alpha1.AddonTemplate{}
			tmpl.Name = claimName + "-tmpl"
			cleanupResource(tmpl)

			By("cleaning up remote Addon")
			addon := &addonsv1alpha1.Addon{}
			addon.Name = addonName
			cleanupResource(addon)

			By("cleaning up remote AddonValue with custom label")
			av := &addonsv1alpha1.AddonValue{}
			av.Name = addonName + "-custom-values"
			cleanupResource(av)
		})

		It("should use .Vars shortcut and custom valueLabels", func() {
			varsTemplateName := claimName + "-tmpl"

			By("Creating AddonTemplate using .Vars syntax")
			createTestAddonTemplate(varsTemplateName, varsTemplate)

			By("Creating AddonClaim with custom valueLabels")
			createTestAddonClaim(claimName, testNamespace, varsTemplateName, secretName,
				WithClaimAddon(claimName),
				WithClaimVariables(map[string]string{
					"name":    claimName,
					"version": "6.5.0",
				}),
				WithClaimValuesString("replicaCount: 2\n"),
				WithClaimValueLabels("custom"),
			)

			By("Waiting for AddonSynced=True")
			waitForClaimCondition(claimName, testNamespace, "AddonSynced", metav1.ConditionTrue)

			By("Verifying Addon is created with correct spec via .Vars")
			Eventually(func() error {
				addon, err := getAddon(addonName)
				if err != nil {
					return err
				}
				if addon.Spec.Version != "6.5.0" {
					return fmt.Errorf("expected version=6.5.0, got %s", addon.Spec.Version)
				}
				return nil
			}, timeout, interval).Should(Succeed(), "Addon should be created via .Vars template")

			By("Verifying AddonValue has custom label")
			addonValueName := addonName + "-custom-values"
			Eventually(func() error {
				av := &addonsv1alpha1.AddonValue{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonValueName}, av); err != nil {
					return err
				}
				if av.Labels["addons.in-cloud.io/values"] != "custom" {
					return fmt.Errorf("expected values label=custom, got %s", av.Labels["addons.in-cloud.io/values"])
				}
				return nil
			}, timeout, interval).Should(Succeed(), "AddonValue should have custom valueLabels")
		})
	})

	Context("Error Handling", func() {
		var claimName string

		BeforeEach(func() {
			claimName = uniqueName("claim-err")
		})

		AfterEach(func() {
			By("cleaning up AddonClaim")
			claim := &addonsv1alpha1.AddonClaim{}
			claim.Name = claimName
			claim.Namespace = testNamespace
			cleanupResource(claim)
		})

		It("should set Degraded when template not found", func() {
			By("Creating AddonClaim with non-existent templateRef")
			createTestAddonClaim(claimName, testNamespace, "non-existent-template", secretName,
				WithClaimAddon(claimName),
				WithClaimVariables(map[string]string{
					"name":    claimName,
					"version": "6.5.0",
				}),
			)

			By("Waiting for TemplateRendered=False")
			waitForClaimCondition(claimName, testNamespace, "TemplateRendered", metav1.ConditionFalse)

			By("Waiting for Degraded=True")
			waitForClaimCondition(claimName, testNamespace, "Degraded", metav1.ConditionTrue)
		})
	})
})

// jsonFromMap converts a map[string]string to *apiextensionsv1.JSON.
func jsonFromMap(m map[string]string) *apiextensionsv1.JSON {
	data := mustMarshal(m)
	return &apiextensionsv1.JSON{Raw: data}
}

// debugAddonClaimStatus prints debug information about an AddonClaim.
func debugAddonClaimStatus(name, namespace string) {
	claim, err := getAddonClaim(name, namespace)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "Failed to get AddonClaim %s/%s: %v\n", namespace, name, err)
		return
	}
	fmt.Fprintf(GinkgoWriter, "AddonClaim %s/%s status:\n", namespace, name)
	fmt.Fprintf(GinkgoWriter, "  AddonName: %s\n", claim.Spec.Addon.Name)
	fmt.Fprintf(GinkgoWriter, "  Ready: %v\n", claim.Status.Ready != nil && *claim.Status.Ready)
	fmt.Fprintf(GinkgoWriter, "  Deployed: %v\n", claim.Status.Deployed)
	fmt.Fprintf(GinkgoWriter, "  Conditions:\n")
	for _, c := range claim.Status.Conditions {
		fmt.Fprintf(GinkgoWriter, "    - %s: %s (%s: %s)\n", c.Type, c.Status, c.Reason, c.Message)
	}
}
