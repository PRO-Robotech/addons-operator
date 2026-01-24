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

package argocd

import (
	"fmt"
	"maps"
	"reflect"
	"strings"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

const (
	defaultProject       = "default"
	defaultTargetCluster = "https://kubernetes.default.svc"
	inClusterDestination = "in-cluster"

	// Addon GVK for ownerReference - TypeMeta is not populated after Get()
	addonAPIVersion = "addons.in-cloud.io/v1alpha1"
	addonKind       = "Addon"
)

// ApplicationBuilder constructs Argo CD Application resources from Addon specs.
type ApplicationBuilder struct{}

// NewApplicationBuilder creates a new ApplicationBuilder.
func NewApplicationBuilder() *ApplicationBuilder {
	return &ApplicationBuilder{}
}

func (b *ApplicationBuilder) Build(addon *addonsv1alpha1.Addon, namespace string, values map[string]any) (*argocdv1alpha1.Application, error) {
	valuesYAML, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal values to YAML: %w", err)
	}

	app := &argocdv1alpha1.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Application",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "addon-operator",
				"addons.in-cloud.io/addon":     addon.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         addonAPIVersion,
					Kind:               addonKind,
					Name:               addon.Name,
					UID:                addon.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				},
			},
		},
		Spec: argocdv1alpha1.ApplicationSpec{
			Project: b.getProject(addon),
			Source: &argocdv1alpha1.ApplicationSource{
				Chart:          addon.Spec.Chart,
				Path:           addon.Spec.Path,
				RepoURL:        addon.Spec.RepoURL,
				TargetRevision: addon.Spec.Version,
				Helm: &argocdv1alpha1.ApplicationSourceHelm{
					Values: string(valuesYAML),
				},
			},
			Destination:       b.getDestination(addon),
			SyncPolicy:        b.getSyncPolicy(addon),
			IgnoreDifferences: b.getIgnoreDifferences(addon),
		},
	}

	return app, nil
}

func (b *ApplicationBuilder) getProject(addon *addonsv1alpha1.Addon) string {
	if addon.Spec.Backend.Project != "" {
		return addon.Spec.Backend.Project
	}
	return defaultProject
}

func (b *ApplicationBuilder) getDestination(addon *addonsv1alpha1.Addon) argocdv1alpha1.ApplicationDestination {
	dest := argocdv1alpha1.ApplicationDestination{
		Namespace: addon.Spec.TargetNamespace,
	}

	switch {
	case addon.Spec.TargetCluster == inClusterDestination:
		dest.Server = defaultTargetCluster
	case isClusterURL(addon.Spec.TargetCluster):
		dest.Server = addon.Spec.TargetCluster
	default:
		dest.Name = addon.Spec.TargetCluster
	}

	return dest
}

func isClusterURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

func (b *ApplicationBuilder) getSyncPolicy(addon *addonsv1alpha1.Addon) *argocdv1alpha1.SyncPolicy {
	if addon.Spec.Backend.SyncPolicy == nil {
		return nil
	}

	sp := addon.Spec.Backend.SyncPolicy
	result := &argocdv1alpha1.SyncPolicy{}

	if sp.Automated != nil {
		result.Automated = &argocdv1alpha1.SyncPolicyAutomated{
			Prune:      sp.Automated.Prune,
			SelfHeal:   sp.Automated.SelfHeal,
			AllowEmpty: sp.Automated.AllowEmpty,
		}
	}

	if len(sp.SyncOptions) > 0 {
		result.SyncOptions = argocdv1alpha1.SyncOptions(sp.SyncOptions)
	}

	if sp.ManagedNamespaceMetadata != nil {
		result.ManagedNamespaceMetadata = &argocdv1alpha1.ManagedNamespaceMetadata{
			Labels:      sp.ManagedNamespaceMetadata.Labels,
			Annotations: sp.ManagedNamespaceMetadata.Annotations,
		}
	}

	return result
}

func (b *ApplicationBuilder) getIgnoreDifferences(addon *addonsv1alpha1.Addon) []argocdv1alpha1.ResourceIgnoreDifferences {
	if len(addon.Spec.Backend.IgnoreDifferences) == 0 {
		return nil
	}

	result := make([]argocdv1alpha1.ResourceIgnoreDifferences, len(addon.Spec.Backend.IgnoreDifferences))
	for i, diff := range addon.Spec.Backend.IgnoreDifferences {
		result[i] = argocdv1alpha1.ResourceIgnoreDifferences{
			Group:                 diff.Group,
			Kind:                  diff.Kind,
			Name:                  diff.Name,
			Namespace:             diff.Namespace,
			JSONPointers:          diff.JSONPointers,
			JQPathExpressions:     diff.JQPathExpressions,
			ManagedFieldsManagers: diff.ManagedFieldsManagers,
		}
	}
	return result
}

func (b *ApplicationBuilder) UpdateSpec(existing *argocdv1alpha1.Application, addon *addonsv1alpha1.Addon, namespace string, values map[string]any) error {
	desired, err := b.Build(addon, namespace, values)
	if err != nil {
		return err
	}

	existing.Spec = desired.Spec
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	maps.Copy(existing.Labels, desired.Labels)

	return nil
}

func (b *ApplicationBuilder) GetApplicationRef(addon *addonsv1alpha1.Addon, namespace string) *addonsv1alpha1.ApplicationRef {
	return &addonsv1alpha1.ApplicationRef{
		Name:      addon.Name,
		Namespace: namespace,
	}
}

func (b *ApplicationBuilder) NeedsUpdate(existing *argocdv1alpha1.Application, addon *addonsv1alpha1.Addon, namespace string, values map[string]any) (bool, string, error) {
	desired, err := b.Build(addon, namespace, values)
	if err != nil {
		return false, "", err
	}

	for k, v := range desired.Labels {
		if existing.Labels[k] != v {
			return true, fmt.Sprintf("label %s differs: existing=%q, desired=%q", k, existing.Labels[k], v), nil
		}
	}

	if existing.Spec.Project != desired.Spec.Project {
		return true, fmt.Sprintf("project differs: existing=%q, desired=%q", existing.Spec.Project, desired.Spec.Project), nil
	}

	if existing.Spec.Destination != desired.Spec.Destination {
		return true, fmt.Sprintf("destination differs: existing=%+v, desired=%+v", existing.Spec.Destination, desired.Spec.Destination), nil
	}

	if existing.Spec.Source == nil && desired.Spec.Source != nil {
		return true, "existing source is nil, desired is not", nil
	}
	if existing.Spec.Source != nil && desired.Spec.Source != nil {
		if existing.Spec.Source.Chart != desired.Spec.Source.Chart {
			return true, fmt.Sprintf("chart differs: existing=%q, desired=%q", existing.Spec.Source.Chart, desired.Spec.Source.Chart), nil
		}
		if existing.Spec.Source.Path != desired.Spec.Source.Path {
			return true, fmt.Sprintf("path differs: existing=%q, desired=%q", existing.Spec.Source.Path, desired.Spec.Source.Path), nil
		}
		if existing.Spec.Source.RepoURL != desired.Spec.Source.RepoURL {
			return true, fmt.Sprintf("repoURL differs: existing=%q, desired=%q", existing.Spec.Source.RepoURL, desired.Spec.Source.RepoURL), nil
		}
		if existing.Spec.Source.TargetRevision != desired.Spec.Source.TargetRevision {
			return true, fmt.Sprintf("targetRevision differs: existing=%q, desired=%q", existing.Spec.Source.TargetRevision, desired.Spec.Source.TargetRevision), nil
		}

		if existing.Spec.Source.Helm == nil && desired.Spec.Source.Helm != nil {
			return true, "existing helm is nil, desired is not", nil
		}
		if existing.Spec.Source.Helm != nil && desired.Spec.Source.Helm != nil {
			if existing.Spec.Source.Helm.Values != desired.Spec.Source.Helm.Values {
				return true, fmt.Sprintf("helm values differ: existing len=%d, desired len=%d", len(existing.Spec.Source.Helm.Values), len(desired.Spec.Source.Helm.Values)), nil
			}
		}
	}

	if !reflect.DeepEqual(existing.Spec.SyncPolicy, desired.Spec.SyncPolicy) {
		return true, "syncPolicy differs", nil
	}

	if !reflect.DeepEqual(existing.Spec.IgnoreDifferences, desired.Spec.IgnoreDifferences) {
		return true, "ignoreDifferences differs", nil
	}

	return false, "", nil
}
