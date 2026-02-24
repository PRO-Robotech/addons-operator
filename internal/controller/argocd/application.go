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
	"encoding/base64"
	"fmt"
	"maps"
	"reflect"
	"strings"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

const (
	defaultProject       = "default"
	defaultTargetCluster = "https://kubernetes.default.svc"
	inClusterDestination = "in-cluster"

	argocdResourcesFinalizer = "resources-finalizer.argocd.argoproj.io"
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

	source := &argocdv1alpha1.ApplicationSource{
		Chart:          addon.Spec.Chart,
		Path:           addon.Spec.Path,
		RepoURL:        addon.Spec.RepoURL,
		TargetRevision: addon.Spec.Version,
	}

	if addon.Spec.PluginName != "" {
		source.Plugin = buildPluginSource(addon.Spec.PluginName, addon.Spec.ReleaseName, valuesYAML)
	} else {
		source.Helm = buildHelmSource(string(valuesYAML), addon.Spec.ReleaseName)
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
					APIVersion:         addonsv1alpha1.GroupVersion.String(),
					Kind:               addonsv1alpha1.AddonKind,
					Name:               addon.Name,
					UID:                addon.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				},
			},
		},
		Spec: argocdv1alpha1.ApplicationSpec{
			Project:           b.getProject(addon),
			Source:            source,
			Destination:       b.getDestination(addon),
			SyncPolicy:        b.getSyncPolicy(addon),
			IgnoreDifferences: b.getIgnoreDifferences(addon),
		},
	}

	if addon.Spec.Backend.Finalizer != nil && *addon.Spec.Backend.Finalizer {
		app.Finalizers = []string{argocdResourcesFinalizer}
	}

	return app, nil
}

func buildHelmSource(valuesYAML string, releaseName string) *argocdv1alpha1.ApplicationSourceHelm {
	helm := &argocdv1alpha1.ApplicationSourceHelm{
		Values: valuesYAML,
	}
	if releaseName != "" {
		helm.ReleaseName = releaseName
	}

	return helm
}

func buildPluginSource(pluginName string, releaseName string, valuesYAML []byte) *argocdv1alpha1.ApplicationSourcePlugin {
	env := argocdv1alpha1.Env{
		&argocdv1alpha1.EnvEntry{
			Name:  "HELM_VALUES",
			Value: base64.StdEncoding.EncodeToString(valuesYAML),
		},
	}
	if releaseName != "" {
		env = append(env, &argocdv1alpha1.EnvEntry{
			Name:  "RELEASE_NAME",
			Value: releaseName,
		})
	}

	return &argocdv1alpha1.ApplicationSourcePlugin{
		Name: pluginName,
		Env:  env,
	}
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

	if controllerutil.ContainsFinalizer(desired, argocdResourcesFinalizer) {
		controllerutil.AddFinalizer(existing, argocdResourcesFinalizer)
	} else {
		controllerutil.RemoveFinalizer(existing, argocdResourcesFinalizer)
	}

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

	if needsUpdate, reason := b.needsSourceUpdate(existing.Spec.Source, desired.Spec.Source); needsUpdate {
		return true, reason, nil
	}

	if !reflect.DeepEqual(existing.Spec.SyncPolicy, desired.Spec.SyncPolicy) {
		return true, "syncPolicy differs", nil
	}

	if !reflect.DeepEqual(existing.Spec.IgnoreDifferences, desired.Spec.IgnoreDifferences) {
		return true, "ignoreDifferences differs", nil
	}

	desiredFinalizer := controllerutil.ContainsFinalizer(desired, argocdResourcesFinalizer)
	hasFinalizer := controllerutil.ContainsFinalizer(existing, argocdResourcesFinalizer)
	if desiredFinalizer != hasFinalizer {
		return true, "argocd resource finalizer differs", nil
	}

	return false, "", nil
}

// needsSourceUpdate compares two ApplicationSource specs and returns whether they differ.
func (b *ApplicationBuilder) needsSourceUpdate(existing, desired *argocdv1alpha1.ApplicationSource) (bool, string) {
	if existing == nil && desired != nil {
		return true, "existing source is nil, desired is not"
	}
	if existing == nil || desired == nil {
		return false, ""
	}

	if existing.Chart != desired.Chart {
		return true, fmt.Sprintf("chart differs: existing=%q, desired=%q", existing.Chart, desired.Chart)
	}
	if existing.Path != desired.Path {
		return true, fmt.Sprintf("path differs: existing=%q, desired=%q", existing.Path, desired.Path)
	}
	if existing.RepoURL != desired.RepoURL {
		return true, fmt.Sprintf("repoURL differs: existing=%q, desired=%q", existing.RepoURL, desired.RepoURL)
	}
	if existing.TargetRevision != desired.TargetRevision {
		return true, fmt.Sprintf("targetRevision differs: existing=%q, desired=%q", existing.TargetRevision, desired.TargetRevision)
	}

	if needsUpdate, reason := b.needsHelmUpdate(existing.Helm, desired.Helm); needsUpdate {
		return true, reason
	}

	if needsUpdate, reason := b.needsPluginUpdate(existing.Plugin, desired.Plugin); needsUpdate {
		return true, reason
	}

	return false, ""
}

// needsHelmUpdate compares two Helm source specs.
func (b *ApplicationBuilder) needsHelmUpdate(existing, desired *argocdv1alpha1.ApplicationSourceHelm) (bool, string) {
	if existing == nil && desired != nil {
		return true, "existing helm is nil, desired is not"
	}
	if existing != nil && desired == nil {
		return true, "existing helm is not nil, desired is nil"
	}
	if existing == nil {
		return false, ""
	}
	if existing.Values != desired.Values {
		return true, fmt.Sprintf("helm values differ: existing len=%d, desired len=%d", len(existing.Values), len(desired.Values))
	}
	if existing.ReleaseName != desired.ReleaseName {
		return true, fmt.Sprintf("helm releaseName differs: existing=%q, desired=%q", existing.ReleaseName, desired.ReleaseName)
	}

	return false, ""
}

// needsPluginUpdate compares two Plugin source specs.
func (b *ApplicationBuilder) needsPluginUpdate(existing, desired *argocdv1alpha1.ApplicationSourcePlugin) (bool, string) {
	if existing == nil && desired != nil {
		return true, "existing plugin is nil, desired is not"
	}
	if existing != nil && desired == nil {
		return true, "existing plugin is not nil, desired is nil"
	}
	if existing == nil {
		return false, ""
	}
	if !existing.Equals(desired) {
		return true, "plugin differs"
	}

	return false, ""
}
