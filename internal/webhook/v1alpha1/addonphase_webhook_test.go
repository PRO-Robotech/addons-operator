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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

var _ = Describe("AddonPhase Webhook", func() {
	var (
		obj       *addonsv1alpha1.AddonPhase
		validator AddonPhaseCustomValidator
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(addonsv1alpha1.AddToScheme(scheme)).To(Succeed())

		obj = &addonsv1alpha1.AddonPhase{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium",
				Namespace: "default",
			},
			Spec: addonsv1alpha1.AddonPhaseSpec{
				Rules: []addonsv1alpha1.PhaseRule{
					{Name: "dev-phase"},
				},
			},
		}
	})

	Context("When validating AddonPhase creation", func() {
		It("Should reject when Addon does not exist", func() {
			By("creating AddonPhase without corresponding Addon")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			validator = AddonPhaseCustomValidator{Client: fakeClient}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("addon cilium not found"))
		})

		It("Should accept when Addon exists", func() {
			By("creating AddonPhase with corresponding Addon")
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cilium",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "cilium",
					RepoURL:         "https://helm.cilium.io/",
					Version:         "1.14.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "kube-system",
					Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(addon).Build()
			validator = AddonPhaseCustomValidator{Client: fakeClient}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject duplicate rule names", func() {
			By("creating AddonPhase with duplicate rule names")
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cilium",
				},
				Spec: addonsv1alpha1.AddonSpec{
					Chart:           "cilium",
					RepoURL:         "https://helm.cilium.io/",
					Version:         "1.14.0",
					TargetCluster:   "in-cluster",
					TargetNamespace: "kube-system",
					Backend:         addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(addon).Build()
			validator = AddonPhaseCustomValidator{Client: fakeClient}

			obj.Spec.Rules = []addonsv1alpha1.PhaseRule{
				{Name: "same-name"},
				{Name: "same-name"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate rule name: same-name"))
		})
	})

	Context("When validating criterion operator+value", func() {
		It("Should reject criterion with operator requiring value but value missing", func() {
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "cilium"},
				Spec: addonsv1alpha1.AddonSpec{
					Chart: "cilium", RepoURL: "https://helm.cilium.io/", Version: "1.14.0",
					TargetCluster: "in-cluster", TargetNamespace: "kube-system",
					Backend: addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(addon).Build()
			validator = AddonPhaseCustomValidator{Client: fakeClient}

			obj.Spec.Rules = []addonsv1alpha1.PhaseRule{
				{
					Name: "test-rule",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "/status/phase",
							Operator: addonsv1alpha1.OperatorEqual,
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "test",
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator Equal requires a value"))
		})

		It("Should reject criterion with Exists operator and value set", func() {
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "cilium"},
				Spec: addonsv1alpha1.AddonSpec{
					Chart: "cilium", RepoURL: "https://helm.cilium.io/", Version: "1.14.0",
					TargetCluster: "in-cluster", TargetNamespace: "kube-system",
					Backend: addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(addon).Build()
			validator = AddonPhaseCustomValidator{Client: fakeClient}

			obj.Spec.Rules = []addonsv1alpha1.PhaseRule{
				{
					Name: "test-rule",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "/status/phase",
							Operator: addonsv1alpha1.OperatorExists,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "test",
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operator Exists must not have a value"))
		})

		It("Should accept valid criteria with proper operator+value", func() {
			addon := &addonsv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "cilium"},
				Spec: addonsv1alpha1.AddonSpec{
					Chart: "cilium", RepoURL: "https://helm.cilium.io/", Version: "1.14.0",
					TargetCluster: "in-cluster", TargetNamespace: "kube-system",
					Backend: addonsv1alpha1.BackendSpec{Type: "argocd", Namespace: "argocd"},
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(addon).Build()
			validator = AddonPhaseCustomValidator{Client: fakeClient}

			obj.Spec.Rules = []addonsv1alpha1.PhaseRule{
				{
					Name: "test-rule",
					Criteria: []addonsv1alpha1.Criterion{
						{
							JSONPath: "/status/phase",
							Operator: addonsv1alpha1.OperatorEqual,
							Value:    &apiextensionsv1.JSON{Raw: []byte(`"Ready"`)},
						},
						{
							JSONPath: "/status/ready",
							Operator: addonsv1alpha1.OperatorExists,
						},
					},
					Selector: addonsv1alpha1.ValuesSelector{
						Name:        "test",
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When validating AddonPhase update", func() {
		It("Should apply same validation rules on update", func() {
			By("updating AddonPhase when Addon does not exist")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			validator = AddonPhaseCustomValidator{Client: fakeClient}

			oldObj := obj.DeepCopy()

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("addon cilium not found"))
		})
	})
})
