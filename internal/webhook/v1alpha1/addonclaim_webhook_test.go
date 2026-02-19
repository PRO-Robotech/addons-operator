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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

const testValuesString = "key: val"

var _ = Describe("AddonClaim Webhook", func() {
	var (
		obj       *addonsv1alpha1.AddonClaim
		validator AddonClaimCustomValidator
	)

	BeforeEach(func() {
		obj = &addonsv1alpha1.AddonClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-claim",
				Namespace: "default",
			},
			Spec: addonsv1alpha1.AddonClaimSpec{
				Name:          "test",
				Version:       "1.0.0",
				Cluster:       "test-cluster",
				CredentialRef: addonsv1alpha1.CredentialRef{Name: "test-secret"},
				TemplateRef:   addonsv1alpha1.TemplateRef{Name: "test-template"},
			},
		}
		validator = AddonClaimCustomValidator{}
	})

	Context("When validating AddonClaim creation", func() {
		It("Should accept when only values is set", func() {
			By("setting only the values field")
			obj.Name = "test-claim-valid-values"
			obj.Spec.Values = &apiextensionsv1.JSON{Raw: []byte(`{"key":"val"}`)}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should accept when only valuesString is set", func() {
			By("setting only the valuesString field")
			obj.Name = "test-claim-valid-values-string"
			obj.Spec.ValuesString = testValuesString

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should accept when neither values nor valuesString is set", func() {
			By("leaving both values and valuesString empty")
			obj.Name = "test-claim-neither-values"

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject when both values and valuesString are set", func() {
			By("setting both values and valuesString")
			obj.Name = "test-claim-both-values"
			obj.Spec.Values = &apiextensionsv1.JSON{Raw: []byte(`{"key":"val"}`)}
			obj.Spec.ValuesString = testValuesString

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("values and valuesString are mutually exclusive"))
		})

		It("Should accept when values is nil", func() {
			By("leaving values as nil with valuesString set")
			obj.Name = "test-claim-nil-values"
			obj.Spec.Values = nil
			obj.Spec.ValuesString = testValuesString

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should accept when values has empty Raw with valuesString set", func() {
			By("setting values with empty Raw bytes and valuesString")
			obj.Name = "test-claim-empty-raw"
			obj.Spec.Values = &apiextensionsv1.JSON{Raw: []byte{}}
			obj.Spec.ValuesString = testValuesString

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating AddonClaim update", func() {
		It("Should apply same validation rules on update", func() {
			By("updating AddonClaim to have both values and valuesString")
			oldObj := obj.DeepCopy()

			obj.Spec.Values = &apiextensionsv1.JSON{Raw: []byte(`{"key":"val"}`)}
			obj.Spec.ValuesString = testValuesString

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("values and valuesString are mutually exclusive"))
		})

		It("Should accept valid update", func() {
			By("updating AddonClaim with only values set")
			oldObj := obj.DeepCopy()

			obj.Spec.Values = &apiextensionsv1.JSON{Raw: []byte(`{"updated":"true"}`)}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating AddonClaim deletion", func() {
		It("Should always accept deletion", func() {
			By("deleting a valid AddonClaim")
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
