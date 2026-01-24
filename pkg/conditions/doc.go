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

// Package conditions provides a reusable condition manager for Kubernetes operators.
//
// The package implements a state machine pattern over standard Kubernetes conditions
// (metav1.Condition) with three primary states: Ready, Progressing, and Degraded.
// This pattern is common in Kubernetes operators and follows the conventions
// established by the Kubernetes community.
//
// # State Machine
//
// The condition manager enforces a state machine where exactly one of the
// primary conditions has Status=True at any time:
//
//   - Ready: Resource is fully reconciled and operating normally
//   - Progressing: Reconciliation is actively in progress
//   - Degraded: An error occurred that prevents normal operation
//
// Typical state flow:
//
//	┌─────────────┐
//	│ Progressing │◄──── Initial state
//	└──────┬──────┘
//	       │
//	       ▼
//	┌─────────────┐     ┌──────────┐
//	│   Ready     │◄───►│ Degraded │
//	└─────────────┘     └──────────┘
//
// # Basic Usage
//
//	func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	    var resource myv1.MyResource
//	    if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
//	        return ctrl.Result{}, client.IgnoreNotFound(err)
//	    }
//
//	    // Create condition manager
//	    cm := conditions.New(&resource.Status.Conditions, resource.Generation)
//	    cm.EnsureAllConditions()
//
//	    // Reconciliation logic
//	    if err := r.doSomething(ctx, &resource); err != nil {
//	        cm.SetDegraded("ReconciliationFailed", "ProcessingError", err.Error())
//	        return ctrl.Result{RequeueAfter: time.Minute}, r.Status().Update(ctx, &resource)
//	    }
//
//	    // Success
//	    cm.SetReady(conditions.ReasonFullyReconciled, "All systems operational")
//	    return ctrl.Result{}, r.Status().Update(ctx, &resource)
//	}
//
// # Operational Conditions
//
// Beyond the primary state machine, you can set additional conditions
// for observability and debugging:
//
//	cm.SetCondition("DatabaseConnected", true, "Connected", "Connection established")
//	cm.SetCondition("CacheWarmed", false, "Warming", "Loading cache entries")
//
// These operational conditions don't affect the primary state machine
// but provide granular status information.
//
// # Customization
//
// The manager can be customized using functional options:
//
//	// Custom time function for testing
//	cm := conditions.New(conds, gen, conditions.WithTimeFunc(func() time.Time {
//	    return fixedTime
//	}))
//
//	// Custom primary conditions
//	cm := conditions.New(conds, gen, conditions.WithPrimaryConditions(
//	    "Available", "Progressing", "Degraded",
//	))
//
// # Guarantees
//
// The Manager provides the following guarantees:
//
//   - All primary conditions always exist with explicit True/False status (never Unknown)
//   - LastTransitionTime only changes when status actually changes
//   - ObservedGeneration is set on all conditions for staleness detection
//   - Consistent reason and message formatting across transitions
package conditions
