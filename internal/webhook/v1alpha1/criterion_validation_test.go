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

var _ = Describe("Criterion Validation", func() {
	Context("operator + value consistency", func() {
		It("Should accept Equal with value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorEqual,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
			}
			Expect(validateCriterion(criterion, "test")).To(Succeed())
		})

		It("Should reject Equal without value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorEqual,
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator Equal requires a value"))
		})

		It("Should reject NotEqual without value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorNotEqual,
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator NotEqual requires a value"))
		})

		It("Should reject In without value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorIn,
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator In requires a value"))
		})

		It("Should reject GreaterThan without value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.replicas",
				Operator: addonsv1alpha1.OperatorGreaterThan,
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator GreaterThan requires a value"))
		})

		It("Should reject Matches without value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.version",
				Operator: addonsv1alpha1.OperatorMatches,
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator Matches requires a value"))
		})

		It("Should accept Exists without value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorExists,
			}
			Expect(validateCriterion(criterion, "test")).To(Succeed())
		})

		It("Should reject Exists with value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorExists,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator Exists must not have a value"))
		})

		It("Should accept NotExists without value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorNotExists,
			}
			Expect(validateCriterion(criterion, "test")).To(Succeed())
		})

		It("Should reject NotExists with value", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorNotExists,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator NotExists must not have a value"))
		})
	})

	Context("JSONPath syntax validation", func() {
		It("Should reject invalid jsonPath without leading dot", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "status/phase",
				Operator: addonsv1alpha1.OperatorEqual,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid jsonPath"))
		})

		It("Should reject invalid jsonPath expression", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$[invalid",
				Operator: addonsv1alpha1.OperatorEqual,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid jsonPath"))
		})

		It("Should accept valid jsonPath", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.conditions[?@.type=='Ready'].status",
				Operator: addonsv1alpha1.OperatorEqual,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"True"`)},
			}
			Expect(validateCriterion(criterion, "test")).To(Succeed())
		})
	})

	Context("Matches regex validation", func() {
		It("Should reject Matches with invalid regex", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.version",
				Operator: addonsv1alpha1.OperatorMatches,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"[invalid"`)},
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid regex pattern for Matches operator"))
		})

		It("Should accept Matches with valid regex", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.version",
				Operator: addonsv1alpha1.OperatorMatches,
				Value:    &apiextensionsv1.JSON{Raw: []byte(`"^v[0-9]+"`)},
			}
			Expect(validateCriterion(criterion, "test")).To(Succeed())
		})
	})

	Context("CriterionSource validation", func() {
		It("Should accept source with name only", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorExists,
				Source: &addonsv1alpha1.CriterionSource{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "my-config",
				},
			}
			Expect(validateCriterion(criterion, "test")).To(Succeed())
		})

		It("Should accept source with labelSelector only", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorExists,
				Source: &addonsv1alpha1.CriterionSource{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			}
			Expect(validateCriterion(criterion, "test")).To(Succeed())
		})

		It("Should reject source with both name and labelSelector", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorExists,
				Source: &addonsv1alpha1.CriterionSource{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "my-config",
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name and labelSelector are mutually exclusive"))
		})

		It("Should reject source with neither name nor labelSelector", func() {
			criterion := addonsv1alpha1.Criterion{
				JSONPath: "$.status.phase",
				Operator: addonsv1alpha1.OperatorExists,
				Source: &addonsv1alpha1.CriterionSource{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
			}
			err := validateCriterion(criterion, "test")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must specify either name or labelSelector"))
		})
	})

	Context("validateCriteria batch validation", func() {
		It("Should accept empty criteria", func() {
			Expect(validateCriteria(nil, "test")).To(Succeed())
		})

		It("Should report the index of the failing criterion", func() {
			criteria := []addonsv1alpha1.Criterion{
				{
					JSONPath: "$.status.phase",
					Operator: addonsv1alpha1.OperatorEqual,
					Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
				},
				{
					JSONPath: "$.status.ready",
					Operator: addonsv1alpha1.OperatorEqual,
					// Missing Value
				},
			}
			err := validateCriteria(criteria, "spec.rules[0].criteria")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.rules[0].criteria[1]"))
		})
	})
})
