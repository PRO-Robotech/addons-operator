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

package conditions

// Primary condition types following Kubernetes conventions.
// These form a state machine where exactly one of Ready/Progressing/Degraded
// should have Status=True at any given time.
//
// State transitions:
//   - Progressing: Active reconciliation in progress
//   - Degraded: Error state, reconciliation failed
//   - Ready: Successfully reconciled and operating normally
const (
	// TypeReady indicates the resource is fully reconciled and operating normally.
	// When Ready=True, the resource has achieved its desired state.
	TypeReady = "Ready"

	// TypeProgressing indicates reconciliation is actively in progress.
	// When Progressing=True, the controller is working toward the desired state.
	TypeProgressing = "Progressing"

	// TypeDegraded indicates an error state that prevents normal operation.
	// When Degraded=True, human intervention may be required.
	TypeDegraded = "Degraded"
)

// Standard reasons for the Ready condition.
const (
	// ReasonFullyReconciled indicates the resource has been fully reconciled
	// and all desired state has been achieved.
	ReasonFullyReconciled = "FullyReconciled"

	// ReasonInitializing indicates the resource is being initialized
	// and has not yet been fully reconciled.
	ReasonInitializing = "Initializing"

	// ReasonDeleting indicates the resource is being deleted.
	ReasonDeleting = "Deleting"
)

// Standard reasons for the Progressing condition.
const (
	// ReasonReconciling indicates active reconciliation is in progress.
	ReasonReconciling = "Reconciling"

	// ReasonComplete indicates reconciliation has completed.
	// Used when Progressing=False after successful reconciliation.
	ReasonComplete = "Complete"
)

// Standard reasons for the Degraded condition.
const (
	// ReasonHealthy indicates no degradation - the resource is healthy.
	// Used when Degraded=False.
	ReasonHealthy = "Healthy"

	// ReasonError indicates a generic error occurred.
	ReasonError = "Error"
)
