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

package addonphase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/conditions"
	"addons-operator/internal/controller/values"
)

func TestLatchDeployedRules(t *testing.T) {
	current := []addonsv1alpha1.ValuesSelector{{
		Name:        "current",
		Priority:    10,
		MatchLabels: map[string]string{"addons.in-cloud.io/current": "true"},
	}}
	previous := []addonsv1alpha1.ValuesSelector{{
		Name:        "previous",
		Priority:    10,
		MatchLabels: map[string]string{"addons.in-cloud.io/previous": "true"},
	}}
	syncedHealthy := []metav1.Condition{
		{Type: conditions.TypeSynced, Status: metav1.ConditionTrue},
		{Type: conditions.TypeHealthy, Status: metav1.ConditionTrue},
	}
	staleSynced := []metav1.Condition{
		{Type: conditions.TypeSynced, Status: metav1.ConditionFalse},
		{Type: conditions.TypeHealthy, Status: metav1.ConditionTrue},
	}

	tests := []struct {
		name       string
		observed   string
		conditions []metav1.Condition
		want       bool
	}{
		{
			name:       "conditions never evaluated with phase selectors",
			observed:   "",
			conditions: syncedHealthy,
			want:       false,
		},
		{
			name:       "conditions evaluated with previous selectors",
			observed:   values.SelectorsHash(previous),
			conditions: syncedHealthy,
			want:       false,
		},
		{
			name:       "current selectors not yet synced",
			observed:   values.SelectorsHash(current),
			conditions: staleSynced,
			want:       false,
		},
		{
			name:       "current selectors synced and healthy",
			observed:   values.SelectorsHash(current),
			conditions: syncedHealthy,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addon := &addonsv1alpha1.Addon{Status: addonsv1alpha1.AddonStatus{
				PhaseValuesSelector:             current,
				ObservedPhaseValuesSelectorHash: tt.observed,
				Conditions:                      tt.conditions,
			}}
			statuses := []addonsv1alpha1.RuleStatus{{Name: "rule", Matched: true}}

			latchDeployedRules(statuses, current, addon)

			assert.Equal(t, tt.want, statuses[0].Deployed)
		})
	}
}
