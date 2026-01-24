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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("AddonValue Webhook", func() {
	var (
		obj       *addonsv1alpha1.AddonValue
		validator AddonValueCustomValidator
	)

	BeforeEach(func() {
		obj = &addonsv1alpha1.AddonValue{}
		validator = AddonValueCustomValidator{}
	})

	Context("When validating AddonValue creation", func() {
		It("Should warn when labels are missing", func() {
			By("creating AddonValue without labels")
			obj.ObjectMeta = metav1.ObjectMeta{
				Name:      "test-value",
				Namespace: "default",
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("missing labels for matching"))
		})

		It("Should warn when addon label is missing", func() {
			By("creating AddonValue with labels but without addon label")
			obj.ObjectMeta = metav1.ObjectMeta{
				Name:      "test-value",
				Namespace: "default",
				Labels: map[string]string{
					"env": "dev",
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring(addonLabelKey))
		})

		It("Should not warn when addon label is present", func() {
			By("creating AddonValue with proper addon label")
			obj.ObjectMeta = metav1.ObjectMeta{
				Name:      "test-value",
				Namespace: "default",
				Labels: map[string]string{
					addonLabelKey: "cilium",
				},
			}

			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	Context("When validating AddonValue update", func() {
		It("Should apply same warning rules on update", func() {
			By("updating AddonValue without addon label")
			oldObj := &addonsv1alpha1.AddonValue{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-value",
					Labels: map[string]string{addonLabelKey: "cilium"},
				},
			}
			obj.ObjectMeta = metav1.ObjectMeta{
				Name:   "test-value",
				Labels: map[string]string{"env": "dev"},
			}

			warnings, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring(addonLabelKey))
		})
	})
})
