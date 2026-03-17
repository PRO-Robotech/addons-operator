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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	addonclaim "addons-operator/internal/controller/addonclaim"
)

const claimTestTemplate = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ index .Values.spec.variables "name" }}
spec:
  chart: {{ index .Values.spec.variables "name" }}
  repoURL: https://charts.example.com
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
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

	makeVariables := func(name, version, cluster string) *apiextensionsv1.JSON {
		return &apiextensionsv1.JSON{
			Raw: []byte(`{"name":"` + name + `","version":"` + version + `","cluster":"` + cluster + `"}`),
		}
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
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(addonName, "1.0.0", "test-cluster"),
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
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(addonName, "1.0.0", "test-cluster"),
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

	It("should create AddonValue with custom valueLabels", func() {
		templateName := uniqueName("lbl-tmpl")
		secretName := uniqueName("lbl-secret")
		claimName := uniqueName("lbl-claim")
		addonName := uniqueName("lbl-addon")

		By("Creating the AddonTemplate")
		createTestAddonTemplate(templateName, claimTestTemplate)

		By("Creating the credential Secret with kubeconfig")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating the AddonClaim with custom valueLabels")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(addonName, "1.0.0", "test-cluster"),
			ValuesString:  "key: value",
			ValueLabels:   "custom",
		})

		By("Waiting for AddonSynced=True")
		waitForClaimCondition(claimName, addonclaim.TypeAddonSynced, metav1.ConditionTrue)

		By("Verifying AddonValue exists with custom label name")
		addonValueName := addonName + "-custom-values"
		av := &addonsv1alpha1.AddonValue{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: addonValueName}, av)
		}, timeout, interval).Should(Succeed())

		By("Verifying AddonValue labels use custom value")
		Expect(av.Labels).To(HaveKeyWithValue("addons.in-cloud.io/addon", addonName))
		Expect(av.Labels).To(HaveKeyWithValue("addons.in-cloud.io/values", "custom"))

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
			Addon:         addonsv1alpha1.AddonIdentity{Name: uniqueName("no-tmpl-addon")},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: "nonexistent-template"},
			Variables:     makeVariables(uniqueName("no-tmpl-addon"), "1.0.0", "test-cluster"),
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
			Addon:         addonsv1alpha1.AddonIdentity{Name: uniqueName("no-sec-addon")},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: "nonexistent-secret"},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(uniqueName("no-sec-addon"), "1.0.0", "test-cluster"),
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
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(addonName, "1.0.0", "test-cluster"),
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

	It("should update Addon when variables change", func() {
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
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(addonName, "1.0.0", "test-cluster"),
		})

		By("Waiting for Addon with version 1.0.0")
		Eventually(func() string {
			addon := &addonsv1alpha1.Addon{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, addon); err != nil {
				return ""
			}

			return addon.Spec.Version
		}, claimTimeout, interval).Should(Equal("1.0.0"))

		By("Updating AddonClaim variables with version 2.0.0")
		Eventually(func() error {
			claim := &addonsv1alpha1.AddonClaim{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, claim); err != nil {
				return err
			}
			claim.Spec.Variables = makeVariables(addonName, "2.0.0", "test-cluster")

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

	It("should sync labels and annotations from template on update", func() {
		secretName := uniqueName("meta-secret")
		claimName := uniqueName("meta-claim")
		addonName := uniqueName("meta-addon")

		tmplWithMeta := func(env string) string {
			return `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: ignored
  labels:
    env: ` + env + `
  annotations:
    note: ` + env + `
spec:
  chart: {{ index .Values.spec.variables "name" }}
  repoURL: https://charts.example.com
  version: "{{ index .Values.spec.variables "version" }}"
  targetCluster: "{{ index .Values.spec.variables "cluster" }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`
		}

		tmplNameV1 := uniqueName("meta-tmpl-v1")
		tmplNameV2 := uniqueName("meta-tmpl-v2")

		By("Creating templates with different metadata")
		createTestAddonTemplate(tmplNameV1, tmplWithMeta("staging"))
		createTestAddonTemplate(tmplNameV2, tmplWithMeta("production"))

		By("Creating credential Secret")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating AddonClaim with v1 template")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: tmplNameV1},
			Variables:     makeVariables(addonName, "1.0.0", "test-cluster"),
		})

		By("Waiting for Addon with staging labels and annotations")
		Eventually(func() string {
			addon := &addonsv1alpha1.Addon{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, addon); err != nil {
				return ""
			}

			return addon.Labels["env"]
		}, claimTimeout, interval).Should(Equal("staging"))

		addon := &addonsv1alpha1.Addon{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, addon)).To(Succeed())
		Expect(addon.Annotations["note"]).To(Equal("staging"))

		By("Switching AddonClaim to v2 template")
		Eventually(func() error {
			claim := &addonsv1alpha1.AddonClaim{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, claim); err != nil {
				return err
			}
			claim.Spec.TemplateRef.Name = tmplNameV2

			return k8sClient.Update(ctx, claim)
		}, timeout, interval).Should(Succeed())

		By("Waiting for Addon labels and annotations to be updated to production")
		Eventually(func() string {
			a := &addonsv1alpha1.Addon{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, a); err != nil {
				return ""
			}

			return a.Labels["env"]
		}, claimTimeout, interval).Should(Equal("production"))

		Eventually(func() string {
			a := &addonsv1alpha1.Addon{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, a); err != nil {
				return ""
			}

			return a.Annotations["note"]
		}, claimTimeout, interval).Should(Equal("production"))

		By("Cleanup")
		deleteAddonClaim(claimName, testNamespace)
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, &addonsv1alpha1.AddonClaim{})

			return apierrors.IsNotFound(err)
		}, claimTimeout, interval).Should(BeTrue())
		deleteAddonTemplate(tmplNameV1)
		deleteAddonTemplate(tmplNameV2)
		deleteSecret(secretName, testNamespace)
		deleteAddon(addonName)
	})

	It("should use spec.addon.name regardless of template rendering", func() {
		templateName := uniqueName("override-tmpl")
		secretName := uniqueName("override-secret")
		claimName := uniqueName("override-claim")
		addonName := uniqueName("override-addon")

		By("Creating the AddonTemplate")
		createTestAddonTemplate(templateName, claimTestTemplate)

		By("Creating the credential Secret with kubeconfig")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating the AddonClaim with spec.addon.name different from template-rendered name")
		templateRenderedName := uniqueName("tmpl-rendered")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(templateRenderedName, "1.0.0", "test-cluster"),
		})

		By("Waiting for AddonSynced=True")
		waitForClaimCondition(claimName, addonclaim.TypeAddonSynced, metav1.ConditionTrue)

		By("Verifying Addon is created with spec.addon.name, not template-rendered name")
		addon := &addonsv1alpha1.Addon{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: addonName}, addon)
		}, claimTimeout, interval).Should(Succeed())

		By("Verifying no addon was created with the template-rendered name")
		err := k8sClient.Get(ctx, types.NamespacedName{Name: templateRenderedName}, &addonsv1alpha1.Addon{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "addon with template-rendered name should not exist")

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

	It("should populate externalStatus when annotation is present", func() {
		templateName := uniqueName("ext-tmpl")
		secretName := uniqueName("ext-secret")
		claimName := uniqueName("ext-claim")
		addonName := uniqueName("ext-addon")

		By("Creating the AddonTemplate")
		createTestAddonTemplate(templateName, claimTestTemplate)

		By("Creating the credential Secret with kubeconfig")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating the AddonClaim with external-status annotation")
		claim := &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: testNamespace,
				Annotations: map[string]string{
					"external-status/type": "controlplane",
				},
			},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
				CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
				Variables:     makeVariables(addonName, "1.5.0", "ext-cluster"),
				Version:       "1.5.0",
			},
		}
		Expect(k8sClient.Create(ctx, claim)).To(Succeed())

		By("Waiting for AddonSynced=True")
		waitForClaimCondition(claimName, addonclaim.TypeAddonSynced, metav1.ConditionTrue)

		By("Verifying CAPI flat fields are populated")
		Eventually(func() bool {
			c := &addonsv1alpha1.AddonClaim{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, c); err != nil {
				return false
			}

			return c.Status.ExternalManagedControlPlane != nil
		}, claimTimeout, interval).Should(BeTrue())

		By("Verifying CAPI status fields")
		c := &addonsv1alpha1.AddonClaim{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, c)).To(Succeed())
		Expect(c.Status.ExternalManagedControlPlane).To(HaveValue(BeTrue()))
		Expect(c.Status.Version).To(Equal("1.5.0"))
		Expect(c.Status.Initialized).NotTo(BeNil())
		Expect(c.Status.Initialization).NotTo(BeNil())

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

	It("should not populate externalStatus without annotation", func() {
		templateName := uniqueName("noext-tmpl")
		secretName := uniqueName("noext-secret")
		claimName := uniqueName("noext-claim")
		addonName := uniqueName("noext-addon")

		By("Creating the AddonTemplate")
		createTestAddonTemplate(templateName, claimTestTemplate)

		By("Creating the credential Secret with kubeconfig")
		kubeconfigBytes := restConfigToKubeconfig(cfg)
		createTestSecret(secretName, testNamespace, map[string][]byte{
			"value": kubeconfigBytes,
		})

		By("Creating the AddonClaim without annotation")
		createTestAddonClaim(claimName, testNamespace, addonsv1alpha1.AddonClaimSpec{
			Addon:         addonsv1alpha1.AddonIdentity{Name: addonName},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: secretName},
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateName},
			Variables:     makeVariables(addonName, "1.0.0", "test-cluster"),
		})

		By("Waiting for AddonSynced=True")
		waitForClaimCondition(claimName, addonclaim.TypeAddonSynced, metav1.ConditionTrue)

		By("Verifying CAPI fields are nil")
		claim := &addonsv1alpha1.AddonClaim{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: testNamespace}, claim)).To(Succeed())
		Expect(claim.Status.ExternalManagedControlPlane).To(BeNil())
		Expect(claim.Status.Initialized).To(BeNil())
		Expect(claim.Status.Initialization).To(BeNil())

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
