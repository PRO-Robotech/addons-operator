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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	addonclaim "addons-operator/internal/controller/addonclaim"
)

const claimTestTemplate = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name }}
spec:
  chart: {{ .Values.spec.name }}
  repoURL: https://charts.example.com
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

var _ = Describe("AddonClaim Controller", func() {
	const testNamespace = "tenant-test"

	// claimTimeout is longer than the default timeout because AddonClaim tests
	// involve remote resource sync which may encounter transient conflicts with
	// the Addon controller (both operate on the same Addon in envtest).
	// The degraded requeue interval is 60s, so we need at least 90s.
	const claimTimeout = 90 * time.Second

	// waitForClaimCondition waits for an AddonClaim condition with the longer claim timeout.
	waitForClaimCondition := func(name, condType string, status metav1.ConditionStatus) {
		EventuallyWithOffset(1, func() metav1.ConditionStatus {
			claim := &addonsv1alpha1.AddonClaim{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, claim)
			if err != nil {
				return metav1.ConditionUnknown
			}
			for _, c := range claim.Status.Conditions {
				if c.Type == condType {
					return c.Status
				}
			}

			return metav1.ConditionUnknown
		}, claimTimeout, interval).Should(Equal(status))
	}

	It("should create Addon in remote cluster from AddonClaim", func() {
		templateName := uniqueName("claim-tmpl")
		secretName := uniqueName("claim-secret")
		claimName := uniqueName("claim-create")
		addonName := uniqueName("addon-create")

		By("Creating the AddonTemplate")
		createTestAddonTemplate(templateName, claimTestTemplate)

		By("Creating the credential Secret with kubeconfig")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating the AddonClaim")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Name:          addonName,
			Version:       "1.0.0",
			Cluster:       "test-cluster",
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
		})

		By("Waiting for TemplateRendered=True")
		waitForClaimCondition(claimName, addonclaim.TypeTemplateRendered, metav1.ConditionTrue)

		By("Waiting for RemoteConnected=True")
		waitForClaimCondition(claimName, addonclaim.TypeRemoteConnected, metav1.ConditionTrue)

		By("Waiting for AddonSynced=True")
		waitForClaimCondition(claimName, addonclaim.TypeAddonSynced, metav1.ConditionTrue)

		By("Verifying Addon exists in the remote cluster")
		addon := &addonsv1alpha1.Addon{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, addon)
		}, timeout, interval).Should(Succeed())

		By("Verifying Addon spec matches claim")
		Expect(addon.Spec.Chart).To(Equal(addonName))
		Expect(addon.Spec.RepoURL).To(Equal("https://charts.example.com"))
		Expect(addon.Spec.Version).To(Equal("1.0.0"))
		Expect(addon.Spec.TargetCluster).To(Equal("test-cluster"))

		By("Cleanup")
		deleteAddonClaim(claimName, testNamespace)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, &addonsv1alpha1.AddonClaim{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())
		deleteAddonTemplate(templateName)
		deleteSecret(secretName, testNamespace)
		deleteAddon(addonName)
	})

	It("should create AddonValue when values are provided", func() {
		templateName := uniqueName("val-tmpl")
		secretName := uniqueName("val-secret")
		claimName := uniqueName("val-claim")
		addonName := uniqueName("val-addon")

		By("Creating the AddonTemplate")
		createTestAddonTemplate(templateName, claimTestTemplate)

		By("Creating the credential Secret with kubeconfig")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating the AddonClaim with ValuesString")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Name:          addonName,
			Version:       "1.0.0",
			Cluster:       "test-cluster",
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			ValuesString:  "key: value",
		})

		By("Waiting for AddonSynced=True")
		waitForClaimCondition(claimName, addonclaim.TypeAddonSynced, metav1.ConditionTrue)

		By("Verifying AddonValue exists in the remote cluster")
		addonValueName := addonName + "-claim-values"
		av := &addonsv1alpha1.AddonValue{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: addonValueName}, av)
		}, timeout, interval).Should(Succeed())

		By("Verifying AddonValue labels")
		Expect(av.Labels).To(HaveKeyWithValue("addons.in-cloud.io/addon", addonName))
		Expect(av.Labels).To(HaveKeyWithValue("addons.in-cloud.io/values", "claim"))

		By("Verifying AddonValue spec.values")
		Expect(av.Spec.Values).To(Equal("key: value"))

		By("Cleanup")
		deleteAddonClaim(claimName, testNamespace)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, &addonsv1alpha1.AddonClaim{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())
		deleteAddonTemplate(templateName)
		deleteSecret(secretName, testNamespace)
		deleteAddon(addonName)
		deleteAddonValue(addonValueName)
	})

	It("should set Degraded when AddonTemplate not found", func() {
		secretName := uniqueName("deg-tmpl-secret")
		claimName := uniqueName("deg-tmpl-claim")

		By("Creating the credential Secret (no template!)")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating the AddonClaim referencing non-existent template")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Name:          uniqueName("no-tmpl-addon"),
			Version:       "1.0.0",
			Cluster:       "test-cluster",
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: "nonexistent-template"},
		})

		By("Waiting for TemplateRendered=False")
		waitForClaimCondition(claimName, addonclaim.TypeTemplateRendered, metav1.ConditionFalse)

		By("Waiting for Degraded=True")
		waitForClaimCondition(claimName, "Degraded", metav1.ConditionTrue)

		By("Cleanup")
		deleteAddonClaim(claimName, testNamespace)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, &addonsv1alpha1.AddonClaim{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())
		deleteSecret(secretName, testNamespace)
	})

	It("should set Degraded when credential Secret not found", func() {
		templateName := uniqueName("deg-sec-tmpl")
		claimName := uniqueName("deg-sec-claim")

		By("Creating the AddonTemplate (no secret!)")
		createTestAddonTemplate(templateName, claimTestTemplate)

		By("Creating the AddonClaim referencing non-existent secret")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Name:          uniqueName("no-sec-addon"),
			Version:       "1.0.0",
			Cluster:       "test-cluster",
			CredentialRef: addonsv1alpha1.CredentialRef{Name: "nonexistent-secret"},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
		})

		By("Waiting for RemoteConnected=False")
		waitForClaimCondition(claimName, addonclaim.TypeRemoteConnected, metav1.ConditionFalse)

		By("Waiting for Degraded=True")
		waitForClaimCondition(claimName, "Degraded", metav1.ConditionTrue)

		By("Cleanup")
		deleteAddonClaim(claimName, testNamespace)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, &addonsv1alpha1.AddonClaim{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())
		deleteAddonTemplate(templateName)
	})

	It("should delete remote resources when AddonClaim is deleted", func() {
		templateName := uniqueName("del-tmpl")
		secretName := uniqueName("del-secret")
		claimName := uniqueName("del-claim")
		addonName := uniqueName("del-addon")

		By("Creating the full setup")
		createTestAddonTemplate(templateName, claimTestTemplate)
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Name:          addonName,
			Version:       "1.0.0",
			Cluster:       "test-cluster",
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
		})

		By("Waiting for Addon to appear")
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, &addonsv1alpha1.Addon{})
		}, claimTimeout, interval).Should(Succeed())

		By("Deleting the AddonClaim")
		deleteAddonClaim(claimName, testNamespace)

		By("Waiting for Addon to be deleted from remote cluster")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, &addonsv1alpha1.Addon{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())

		By("Waiting for AddonClaim to be fully deleted")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, &addonsv1alpha1.AddonClaim{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())

		By("Cleanup remaining resources")
		deleteAddonTemplate(templateName)
		deleteSecret(secretName, testNamespace)
	})

	It("should update Addon when AddonClaim version changes", func() {
		templateName := uniqueName("upd-tmpl")
		secretName := uniqueName("upd-secret")
		claimName := uniqueName("upd-claim")
		addonName := uniqueName("upd-addon")

		By("Creating the full setup with version 1.0.0")
		createTestAddonTemplate(templateName, claimTestTemplate)
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Name:          addonName,
			Version:       "1.0.0",
			Cluster:       "test-cluster",
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
		})

		By("Waiting for Addon with version 1.0.0")
		Eventually(func() string {
			addon := &addonsv1alpha1.Addon{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, addon); err != nil {
				return ""
			}

			return addon.Spec.Version
		}, claimTimeout, interval).Should(Equal("1.0.0"))

		By("Updating AddonClaim version to 2.0.0")
		Eventually(func() error {
			claim := &addonsv1alpha1.AddonClaim{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, claim); err != nil {
				return err
			}
			claim.Spec.Version = "2.0.0"

			return k8sClient.Update(ctx, claim)
		}, timeout, interval).Should(Succeed())

		By("Waiting for Addon to be updated with version 2.0.0")
		Eventually(func() string {
			addon := &addonsv1alpha1.Addon{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, addon); err != nil {
				return ""
			}

			return addon.Spec.Version
		}, claimTimeout, interval).Should(Equal("2.0.0"))

		By("Cleanup")
		deleteAddonClaim(claimName, testNamespace)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, &addonsv1alpha1.AddonClaim{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())
		deleteAddonTemplate(templateName)
		deleteSecret(secretName, testNamespace)
		deleteAddon(addonName)
	})
})
