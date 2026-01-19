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

import (
	pkgconditions "addons-operator/pkg/conditions"
)

// Re-export generic condition types from pkg/conditions for convenience.
// This allows controllers to use a single import for all condition-related constants.
const (
	TypeReady       = pkgconditions.TypeReady
	TypeProgressing = pkgconditions.TypeProgressing
	TypeDegraded    = pkgconditions.TypeDegraded
)

// Re-export generic reasons from pkg/conditions.
const (
	ReasonFullyReconciled = pkgconditions.ReasonFullyReconciled
	ReasonInitializing    = pkgconditions.ReasonInitializing
	ReasonDeleting        = pkgconditions.ReasonDeleting
	ReasonReconciling     = pkgconditions.ReasonReconciling
	ReasonComplete        = pkgconditions.ReasonComplete
	ReasonHealthy         = pkgconditions.ReasonHealthy
)

// Addon-specific operational condition types.
// These conditions provide granular observability for Addon resources.
const (
	// TypeDependenciesMet indicates initDependencies is satisfied.
	TypeDependenciesMet = "DependenciesMet"
	// TypeValuesResolved indicates values are aggregated and templated.
	TypeValuesResolved = "ValuesResolved"
	// TypeApplicationCreated indicates ArgoCD Application exists.
	TypeApplicationCreated = "ApplicationCreated"
	// TypeSynced indicates ArgoCD Application is synced.
	TypeSynced = "Synced"
	// TypeHealthy indicates ArgoCD Application is healthy.
	TypeHealthy = "Healthy"
)

// Addon-specific Ready condition reasons.
const (
	ReasonDependenciesNotMet = "DependenciesNotMet"
	ReasonValuesNotResolved  = "ValuesNotResolved"
	ReasonApplicationFailed  = "ApplicationFailed"
	ReasonNotSynced          = "NotSynced"
	ReasonUnhealthy          = "Unhealthy"
)

// Addon-specific Progressing condition reasons.
const (
	ReasonWaitingForSync       = "WaitingForSync"
	ReasonWaitingForHealthy    = "WaitingForHealthy"
	ReasonWaitingForDependency = "WaitingForDependency"
	ReasonCreatingApplication  = "CreatingApplication"
	ReasonResolvingValues      = "ResolvingValues"
)

// Addon-specific Degraded condition reasons.
const (
	ReasonValueSourceError = "ValueSourceError"
	ReasonTemplateError    = "TemplateError"
	ReasonApplicationError = "ApplicationError"
	ReasonDegradedHealth   = "DegradedHealth"
	ReasonEvaluationFailed = "EvaluationFailed"
	ReasonPatchFailed      = "PatchFailed"
)

// Common reasons used across different condition types.
const (
	ReasonTargetAddonNotFound = "TargetAddonNotFound"
)
