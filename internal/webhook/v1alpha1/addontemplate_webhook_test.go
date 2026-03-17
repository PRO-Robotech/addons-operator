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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("AddonTemplate Webhook", func() {
	var (
		obj       *addonsv1alpha1.AddonTemplate
		validator AddonTemplateCustomValidator
	)

	BeforeEach(func() {
		obj = &addonsv1alpha1.AddonTemplate{}
		validator = AddonTemplateCustomValidator{}
	})

	Context("When validating AddonTemplate creation", func() {
		It("Should accept a valid template", func() {
			By("setting a proper Go template")
			obj.Spec.Template = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name }}
spec:
  repoURL: https://example.com
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject an empty template", func() {
			By("setting an empty template string")
			obj.Spec.Template = ""

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.template must not be empty"))
		})

		It("Should reject a template with bad syntax", func() {
			By("setting a template with an unknown function")
			obj.Spec.Template = `{{ .Values.spec.name | unknownFunc }}`

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid template"))
		})

		It("Should accept a template with sprig functions", func() {
			By("setting a template using sprig's default function")
			obj.Spec.Template = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name | default "my-addon" | quote }}
spec:
  repoURL: https://example.com
  version: {{ .Values.spec.version | default "1.0.0" | quote }}
  targetCluster: {{ .Values.spec.cluster | default "in-cluster" | quote }}
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating AddonTemplate update", func() {
		It("Should accept a valid template change", func() {
			By("creating a valid old template")
			oldObj := &addonsv1alpha1.AddonTemplate{}
			oldObj.Spec.Template = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name }}
spec:
  repoURL: https://example.com
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: default
  backend:
    type: argocd
    namespace: argocd`

			By("updating to a new valid template")
			obj.Spec.Template = `apiVersion: addons.in-cloud.io/v1alpha1
kind: Addon
metadata:
  name: {{ .Values.spec.name | default "updated-addon" }}
spec:
  repoURL: https://updated.example.com
  version: "{{ .Values.spec.version }}"
  targetCluster: "{{ .Values.spec.cluster }}"
  targetNamespace: kube-system
  backend:
    type: argocd
    namespace: argocd`

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
