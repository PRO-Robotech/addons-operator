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
func newTestConditionManager(conds *[]metav1.Condition, generation int64) *conditions.Manager {
	return conditions.NewManager(conds, generation)
}

func TestStatusTranslator_UpdateConditions_SyncedAndHealthy(t *testing.T) {
	translator := NewStatusTranslator()

	app := &argocdv1alpha1.Application{
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeSynced,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: health.HealthStatusHealthy,
			},
		},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds, 1)
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

	app := &argocdv1alpha1.Application{
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Status: argocdv1alpha1.SyncStatusCodeOutOfSync,
			},
			Health: argocdv1alpha1.HealthStatus{
				Status:  health.HealthStatusDegraded,
				Message: "Pod is crashing",
			},
		},
	}

	conds := []metav1.Condition{}
	cm := newTestConditionManager(&conds, 1)
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

// Helper function
func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}
