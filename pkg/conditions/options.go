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

import "time"

// Option configures a Manager.
type Option func(*Manager)

// WithTimeFunc sets a custom time function.
// This is primarily useful for testing to control time-based behavior.
//
// Example:
//
//	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
//	cm := conditions.New(conds, gen, conditions.WithTimeFunc(func() time.Time {
//	    return fixedTime
//	}))
func WithTimeFunc(fn func() time.Time) Option {
	return func(m *Manager) {
		m.now = fn
	}
}

// WithPrimaryConditions sets custom primary condition types.
// By default, the manager uses Ready, Progressing, and Degraded.
// Use this option to define a different set of primary conditions.
//
// Example:
//
//	cm := conditions.New(conds, gen, conditions.WithPrimaryConditions(
//	    "Available", "Progressing", "Degraded",
//	))
func WithPrimaryConditions(types ...string) Option {
	return func(m *Manager) {
		m.primaryConditions = types
	}
}

// WithDefaultReasons sets default reasons for a specific condition type.
// These reasons are used when initializing conditions in EnsureAllConditions().
//
// Example:
//
//	cm := conditions.New(conds, gen, conditions.WithDefaultReasons(
//	    conditions.TypeReady, "ServiceReady", "ServiceNotReady",
//	))
func WithDefaultReasons(condType, whenTrue, whenFalse string) Option {
	return func(m *Manager) {
		m.defaultReasons[condType] = defaultReasons{
			whenTrue:  whenTrue,
			whenFalse: whenFalse,
		}
	}
}
