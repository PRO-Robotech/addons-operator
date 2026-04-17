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

package status

import (
	"testing"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"addons-operator/internal/controller/conditions"
)

// newTestConditionManager creates a condition manager for testing.
func newTestConditionManager(conds *[]metav1.Condition) *conditions.Manager {
	return conditions.NewManager(conds, 1)
}

// newHelmApp builds a minimal Application
func newHelmApp(syncStatus argocdv1alpha1.SyncStatusCode, healthStatus health.HealthStatusCode, healthMessage string) *argocdv1alpha1.Application {
	newSource := func() argocdv1alpha1.ApplicationSource {
		return argocdv1alpha1.ApplicationSource{
			RepoURL:        "https://charts.example.com",
			Chart:          "my-chart",
			TargetRevision: "1.2.3",
			Helm: &argocdv1alpha1.ApplicationSourceHelm{
				Values:      "key: value\n",
				ReleaseName: "my-release",
			},
		}
	}
	specSource := newSource()

	return &argocdv1alpha1.Application{
		Spec: argocdv1alpha1.ApplicationSpec{
			Source: &specSource,
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: syncStatus,
				ComparedTo: argocdv1alpha1.ComparedTo{
					Source: newSource(),
				},
			},
			Health: argocdv1alpha1.HealthStatus{
				Status:  healthStatus,
				Message: healthMessage,
			},
		},
	}
}

func TestStatusTranslator_UpdateConditions_SyncedAndHealthy(t *testing.T) {
	translator := NewStatusTranslator()
	app := newHelmApp(argocdv1alpha1.SyncStatusCodeSynced, health.HealthStatusHealthy, "")

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	// Check Synced condition is True
	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionTrue, synced.Status)
	assert.Equal(t, "Synced", synced.Reason)

	// Check Healthy condition is True
	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionTrue, healthy.Status)
	assert.Equal(t, "Healthy", healthy.Reason)
}

func TestStatusTranslator_UpdateConditions_OutOfSyncDegraded(t *testing.T) {
	translator := NewStatusTranslator()
	app := newHelmApp(argocdv1alpha1.SyncStatusCodeOutOfSync, health.HealthStatusDegraded, "Pod is crashing")

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	// Check Synced condition is False
	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, "OutOfSync", synced.Reason)

	// Check Healthy condition is False
	healthy := findCondition(conds, conditions.TypeHealthy)
	require.NotNil(t, healthy)
	assert.Equal(t, metav1.ConditionFalse, healthy.Status)
	assert.Equal(t, "Degraded", healthy.Reason)
	assert.Equal(t, "Pod is crashing", healthy.Message)
}

// Regression: after the operator updates spec.source.helm.values, Argo's
// status.sync.status may still read "Synced" against the previous spec until
// Argo re-compares. In that window we must not claim Synced=true, otherwise
// Deployed=true gets latched against a spec Argo has never actually observed.
// This is the scenario Dmitry reproduced by freezing Argo pods and recreating
// the AddonPhase.
func TestStatusTranslator_UpdateConditions_StaleComparedTo_HelmValuesDiffer(t *testing.T) {
	translator := NewStatusTranslator()
	app := newHelmApp(argocdv1alpha1.SyncStatusCodeSynced, health.HealthStatusHealthy, "")
	// Simulate "operator just wrote new values, Argo hasn't re-compared yet"
	// by mutating spec.source.helm.values without touching comparedTo.
	app.Spec.Source.Helm.Values = "key: newvalue\n"

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status, "stale Synced=true must not be trusted")
	assert.Equal(t, reasonStale, synced.Reason)
}

func TestStatusTranslator_UpdateConditions_StaleComparedTo_TargetRevisionDiffers(t *testing.T) {
	translator := NewStatusTranslator()
	app := newHelmApp(argocdv1alpha1.SyncStatusCodeSynced, health.HealthStatusHealthy, "")
	app.Spec.Source.TargetRevision = "9.9.9" // operator bumped chart version, Argo hasn't observed yet

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, reasonStale, synced.Reason)
}

func TestStatusTranslator_UpdateConditions_EmptyComparedTo(t *testing.T) {
	translator := NewStatusTranslator()
	app := newHelmApp(argocdv1alpha1.SyncStatusCodeSynced, health.HealthStatusHealthy, "")
	// Brand-new Application: Argo has not completed a single compare cycle yet.
	app.Status.Sync.ComparedTo = argocdv1alpha1.ComparedTo{}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, reasonStale, synced.Reason)
}

func TestStatusTranslator_UpdateConditions_NilSpecSource(t *testing.T) {
	translator := NewStatusTranslator()
	app := &argocdv1alpha1.Application{
		Spec: argocdv1alpha1.ApplicationSpec{Source: nil},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync:   argocdv1alpha1.SyncStatus{Status: argocdv1alpha1.SyncStatusCodeSynced},
			Health: argocdv1alpha1.HealthStatus{Status: health.HealthStatusHealthy},
		},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, reasonStale, synced.Reason)
}

func TestStatusTranslator_UpdateConditions_MultiSourceNotSupported(t *testing.T) {
	translator := NewStatusTranslator()
	app := newHelmApp(argocdv1alpha1.SyncStatusCodeSynced, health.HealthStatusHealthy, "")
	// Someone manually converted the Application into a multi-source one.
	// We can't safely reason about it — refuse to report Synced.
	app.Spec.Sources = argocdv1alpha1.ApplicationSources{
		{RepoURL: "https://charts.example.com", Chart: "extra", TargetRevision: "1.0.0"},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, reasonStale, synced.Reason)
	assert.Contains(t, synced.Message, "multi-source")
}

func TestStatusTranslator_UpdateConditions_PluginSourceObserved(t *testing.T) {
	translator := NewStatusTranslator()

	src := argocdv1alpha1.ApplicationSource{
		RepoURL:        "https://charts.example.com",
		Chart:          "my-chart",
		TargetRevision: "1.0.0",
		Plugin: &argocdv1alpha1.ApplicationSourcePlugin{
			Name: "helm-values",
			Env: argocdv1alpha1.Env{
				{Name: "HELM_VALUES", Value: "a2V5OiB2YWx1ZQo="},
				{Name: "RELEASE_NAME", Value: "my-release"},
			},
		},
	}
	specSource := src

	app := &argocdv1alpha1.Application{
		Spec: argocdv1alpha1.ApplicationSpec{Source: &specSource},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status:     argocdv1alpha1.SyncStatusCodeSynced,
				ComparedTo: argocdv1alpha1.ComparedTo{Source: src},
			},
			Health: argocdv1alpha1.HealthStatus{Status: health.HealthStatusHealthy},
		},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionTrue, synced.Status)
}

func TestStatusTranslator_UpdateConditions_PluginEnvDiffers(t *testing.T) {
	translator := NewStatusTranslator()

	observedSrc := argocdv1alpha1.ApplicationSource{
		RepoURL:        "https://charts.example.com",
		Chart:          "my-chart",
		TargetRevision: "1.0.0",
		Plugin: &argocdv1alpha1.ApplicationSourcePlugin{
			Name: "helm-values",
			Env: argocdv1alpha1.Env{
				{Name: "HELM_VALUES", Value: "OLD=="},
			},
		},
	}
	specSrc := observedSrc
	// spec uses new HELM_VALUES; Argo still stores old one in comparedTo.
	specSrc.Plugin = &argocdv1alpha1.ApplicationSourcePlugin{
		Name: "helm-values",
		Env: argocdv1alpha1.Env{
			{Name: "HELM_VALUES", Value: "NEW=="},
		},
	}

	app := &argocdv1alpha1.Application{
		Spec: argocdv1alpha1.ApplicationSpec{Source: &specSrc},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status:     argocdv1alpha1.SyncStatusCodeSynced,
				ComparedTo: argocdv1alpha1.ComparedTo{Source: observedSrc},
			},
			Health: argocdv1alpha1.HealthStatus{Status: health.HealthStatusHealthy},
		},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds)
	translator.UpdateConditions(cm, app)

	synced := findCondition(conds, conditions.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)
	assert.Equal(t, reasonStale, synced.Reason)
}

// Helper function
func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}

	return nil
}
