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
	"fmt"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"addons-operator/internal/controller/conditions"
)

const (
	reasonUnknown = "Unknown"
)

// StatusTranslator translates Argo CD Application status to Addon conditions.
// It maps Application sync.status and health.status to standardized Kubernetes
// conditions, providing a unified view of addon health.
type StatusTranslator struct{}

// NewStatusTranslator creates a new StatusTranslator.
func NewStatusTranslator() *StatusTranslator {
	return &StatusTranslator{}
}

// Translate converts Application status to Addon conditions.
// Returns conditions for: ApplicationCreated, Synced, Healthy, Ready.
func (t *StatusTranslator) Translate(app *argocdv1alpha1.Application, generation int64) []metav1.Condition {
	result := make([]metav1.Condition, 0, 4)

	// ApplicationCreated condition - always true if we have an Application
	result = append(result, metav1.Condition{
		Type:               conditions.TypeApplicationCreated,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		Reason:             "ApplicationExists",
		Message:            fmt.Sprintf("Application %s/%s exists", app.Namespace, app.Name),
	})

	// Synced condition - maps from sync.status
	syncCondition := t.translateSyncStatus(app, generation)
	result = append(result, syncCondition)

	// Healthy condition - maps from health.status
	healthCondition := t.translateHealthStatus(app, generation)
	result = append(result, healthCondition)

	// Ready condition - composite of Synced AND Healthy
	readyCondition := t.calculateReadyCondition(syncCondition, healthCondition, generation)
	result = append(result, readyCondition)

	return result
}

// translateSyncStatus converts Application sync.status to Synced condition.
func (t *StatusTranslator) translateSyncStatus(app *argocdv1alpha1.Application, generation int64) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditions.TypeSynced,
		ObservedGeneration: generation,
	}

	switch app.Status.Sync.Status {
	case argocdv1alpha1.SyncStatusCodeSynced:
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Synced"
		condition.Message = "Application is synced with the target revision"
	case argocdv1alpha1.SyncStatusCodeOutOfSync:
		condition.Status = metav1.ConditionFalse
		condition.Reason = "OutOfSync"
		condition.Message = "Application is out of sync with the target revision"
	case argocdv1alpha1.SyncStatusCodeUnknown:
		condition.Status = metav1.ConditionUnknown
		condition.Reason = reasonUnknown
		condition.Message = "Sync status is unknown"
	default:
		condition.Status = metav1.ConditionUnknown
		condition.Reason = reasonUnknown
		condition.Message = fmt.Sprintf("Unexpected sync status: %s", app.Status.Sync.Status)
	}

	return condition
}

// translateHealthStatus converts Application health.status to Healthy condition.
func (t *StatusTranslator) translateHealthStatus(app *argocdv1alpha1.Application, generation int64) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditions.TypeHealthy,
		ObservedGeneration: generation,
	}

	switch app.Status.Health.Status {
	case health.HealthStatusHealthy:
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Healthy"
		condition.Message = "All resources are healthy"
	case health.HealthStatusDegraded:
		condition.Status = metav1.ConditionFalse
		condition.Reason = "Degraded"
		condition.Message = t.getHealthMessage(app, "Application health is degraded")
	case health.HealthStatusProgressing:
		condition.Status = metav1.ConditionFalse
		condition.Reason = "Progressing"
		condition.Message = "Application resources are progressing"
	case health.HealthStatusSuspended:
		condition.Status = metav1.ConditionFalse
		condition.Reason = "Suspended"
		condition.Message = "Application is suspended"
	case health.HealthStatusMissing:
		condition.Status = metav1.ConditionFalse
		condition.Reason = "Missing"
		condition.Message = "Application resources are missing"
	case health.HealthStatusUnknown:
		condition.Status = metav1.ConditionUnknown
		condition.Reason = reasonUnknown
		condition.Message = "Health status is unknown"
	default:
		condition.Status = metav1.ConditionUnknown
		condition.Reason = reasonUnknown
		condition.Message = fmt.Sprintf("Unexpected health status: %s", app.Status.Health.Status)
	}

	return condition
}

// getHealthMessage returns the health message from the Application, or a default.
func (t *StatusTranslator) getHealthMessage(app *argocdv1alpha1.Application, defaultMsg string) string {
	if app.Status.Health.Message != "" {
		return app.Status.Health.Message
	}
	return defaultMsg
}

// calculateReadyCondition computes the Ready condition from Synced and Healthy.
// Ready = Synced AND Healthy
func (t *StatusTranslator) calculateReadyCondition(synced, healthy metav1.Condition, generation int64) metav1.Condition {
	condition := metav1.Condition{
		Type:               conditions.TypeReady,
		ObservedGeneration: generation,
	}

	// Both must be True for Ready=True
	if synced.Status == metav1.ConditionTrue && healthy.Status == metav1.ConditionTrue {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Ready"
		condition.Message = "Addon is ready (Synced and Healthy)"
		return condition
	}

	// Determine the reason for not being ready
	condition.Status = metav1.ConditionFalse

	if synced.Status != metav1.ConditionTrue {
		condition.Reason = "NotSynced"
		condition.Message = synced.Message
	} else {
		// Must be unhealthy
		condition.Reason = healthy.Reason
		condition.Message = healthy.Message
	}

	return condition
}

// UpdateConditions updates the Condition Manager with Application status.
// Sets operational conditions: Synced, Healthy.
// The primary conditions (Ready, Progressing, Degraded) are managed by the controller.
func (t *StatusTranslator) UpdateConditions(cm *conditions.Manager, app *argocdv1alpha1.Application) {
	// Update Synced condition
	syncStatus, syncReason, syncMessage := t.getSyncInfo(app)
	cm.SetOperationalCondition(conditions.TypeSynced, syncStatus, syncReason, syncMessage)

	// Update Healthy condition
	healthStatus, healthReason, healthMessage := t.getHealthInfo(app)
	cm.SetOperationalCondition(conditions.TypeHealthy, healthStatus, healthReason, healthMessage)
}

// getSyncInfo extracts sync information from Application.
func (t *StatusTranslator) getSyncInfo(app *argocdv1alpha1.Application) (bool, string, string) {
	switch app.Status.Sync.Status {
	case argocdv1alpha1.SyncStatusCodeSynced:
		return true, "Synced", "Application is synced with the target revision"
	case argocdv1alpha1.SyncStatusCodeOutOfSync:
		return false, "OutOfSync", "Application is out of sync with the target revision"
	default:
		return false, reasonUnknown, fmt.Sprintf("Sync status: %s", app.Status.Sync.Status)
	}
}

// getHealthInfo extracts health information from Application.
func (t *StatusTranslator) getHealthInfo(app *argocdv1alpha1.Application) (bool, string, string) {
	switch app.Status.Health.Status {
	case health.HealthStatusHealthy:
		return true, "Healthy", "All resources are healthy"
	case health.HealthStatusDegraded:
		return false, "Degraded", t.getHealthMessage(app, "Application health is degraded")
	case health.HealthStatusProgressing:
		return false, "Progressing", "Application resources are progressing"
	case health.HealthStatusSuspended:
		return false, "Suspended", "Application is suspended"
	case health.HealthStatusMissing:
		return false, "Missing", "Application resources are missing"
	default:
		return false, reasonUnknown, fmt.Sprintf("Health status: %s", app.Status.Health.Status)
	}
}

// SetCondition sets a condition in the conditions slice, updating LastTransitionTime
// only if the status changed.
func SetCondition(conds *[]metav1.Condition, newCondition metav1.Condition) {
	if conds == nil {
		return
	}

	now := metav1.Now()
	newCondition.LastTransitionTime = now

	for i, existing := range *conds {
		if existing.Type == newCondition.Type {
			// Keep the old transition time if status hasn't changed
			if existing.Status == newCondition.Status {
				newCondition.LastTransitionTime = existing.LastTransitionTime
			}
			(*conds)[i] = newCondition
			return
		}
	}

	// Condition not found, append it
	*conds = append(*conds, newCondition)
}
