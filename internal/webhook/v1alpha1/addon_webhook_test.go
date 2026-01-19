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

var _ = Describe("Addon Webhook", func() {
	var (
		obj       *addonsv1alpha1.Addon
		validator AddonCustomValidator
	)

	BeforeEach(func() {
		obj = &addonsv1alpha1.Addon{}
		validator = AddonCustomValidator{}
	})

	Context("When validating Addon creation", func() {
		It("Should reject unsupported backend type", func() {
			By("setting backend type to 'helm'")
			obj.Spec.Backend.Type = "helm"

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported backend type"))
		})

		It("Should accept argocd backend type", func() {
			By("setting backend type to 'argocd'")
			obj.Spec.Backend.Type = supportedBackendType

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject duplicate selector names", func() {
			By("creating addon with duplicate selector names")
			obj.Spec.Backend.Type = supportedBackendType
			obj.Spec.ValuesSelectors = []addonsv1alpha1.ValuesSelector{
				{Name: "default", Priority: 0, MatchLabels: map[string]string{"env": "dev"}},
				{Name: "default", Priority: 1, MatchLabels: map[string]string{"env": "prod"}},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate selector name: default"))
		})

		It("Should accept unique selector names", func() {
			By("creating addon with unique selector names")
			obj.Spec.Backend.Type = supportedBackendType
			obj.Spec.ValuesSelectors = []addonsv1alpha1.ValuesSelector{
				{Name: "dev", Priority: 0, MatchLabels: map[string]string{"env": "dev"}},
				{Name: "prod", Priority: 1, MatchLabels: map[string]string{"env": "prod"}},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating Addon update", func() {
		It("Should apply same validation rules on update", func() {
			By("updating addon with invalid backend type")
			oldObj := &addonsv1alpha1.Addon{}
			oldObj.Spec.Backend.Type = supportedBackendType

			obj.Spec.Backend.Type = "helm"

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported backend type"))
		})
	})
})
