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

	"addons-operator/internal/controller/conditions"
)

const (
	reasonUnknown = "Unknown"
	reasonStale   = "Stale"
)

// StatusTranslator translates Argo CD Application status to Addon conditions.
// It maps Application sync.status and health.status to standardized Kubernetes
// conditions, providing a unified view of addon health.
type StatusTranslator struct{}

// NewStatusTranslator creates a new StatusTranslator.
func NewStatusTranslator() *StatusTranslator {
	return &StatusTranslator{}
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

// getSyncInfo returns Synced=false with reason "Stale" if Argo has not yet
// compared the Application against the current spec.source — otherwise the
// Sync/Health status reflects a previous spec and would falsely latch Deployed.
func (t *StatusTranslator) getSyncInfo(app *argocdv1alpha1.Application) (bool, string, string) {
	if current, reason := isComparedToCurrent(app); !current {
		return false, reasonStale, reason
	}

	switch app.Status.Sync.Status {
	case argocdv1alpha1.SyncStatusCodeSynced:
		return true, "Synced", "Application is synced with the target revision"
	case argocdv1alpha1.SyncStatusCodeOutOfSync:
		return false, "OutOfSync", "Application is out of sync with the target revision"
	default:
		return false, reasonUnknown, fmt.Sprintf("Sync status: %s", app.Status.Sync.Status)
	}
}

// isComparedToCurrent reports whether status.sync.comparedTo matches the
// current spec.source. Uses Argo's own ApplicationSource.Equals.
func isComparedToCurrent(app *argocdv1alpha1.Application) (bool, string) {
	if len(app.Spec.Sources) > 0 || len(app.Status.Sync.ComparedTo.Sources) > 0 {
		return false, "multi-source Applications are not supported by this operator"
	}

	if app.Spec.Source == nil {
		return false, "Application spec.source is empty"
	}

	observed := app.Status.Sync.ComparedTo.Source
	if observed.RepoURL == "" && observed.Chart == "" && observed.Path == "" {
		return false, "Argo has not yet compared the Application against its spec"
	}

	if !app.Spec.Source.Equals(&observed) {
		return false, "Argo has not yet observed the current spec.source"
	}

	return true, ""
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

// getHealthMessage returns the health message from the Application, or a default.
func (t *StatusTranslator) getHealthMessage(app *argocdv1alpha1.Application, defaultMsg string) string {
	if app.Status.Health.Message != "" {
		return app.Status.Health.Message
	}

	return defaultMsg
}
