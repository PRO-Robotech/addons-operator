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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgconditions "addons-operator/pkg/conditions"
)

// Manager wraps pkg/conditions.Manager with Addon-specific convenience methods.
// It provides backward-compatible API while delegating to the reusable package.
type Manager struct {
	*pkgconditions.Manager
}

// NewManager creates a new condition manager for the given conditions slice.
// Generation should be the resource's metadata.generation.
func NewManager(conditions *[]metav1.Condition, generation int64) *Manager {
	return &Manager{
		Manager: pkgconditions.New(conditions, generation),
	}
}

// NewManagerWithTimeFunc creates a new condition manager with a custom time function.
// This is primarily useful for testing.
func NewManagerWithTimeFunc(conditions *[]metav1.Condition, generation int64, now func() time.Time) *Manager {
	return &Manager{
		Manager: pkgconditions.New(conditions, generation, pkgconditions.WithTimeFunc(now)),
	}
}

// SetOperationalCondition sets a supplementary condition for observability.
// These conditions provide granular debugging information.
// This is an alias for SetCondition to maintain backward compatibility.
func (m *Manager) SetOperationalCondition(condType string, status bool, reason, message string) {
	m.SetCondition(condType, status, reason, message)
}
