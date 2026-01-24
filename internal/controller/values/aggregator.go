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

package values

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// AddonLabelPrefix is the prefix for all addon-related labels.
// Only labels with this prefix are considered for exact match.
const AddonLabelPrefix = "addons.in-cloud.io/"

// Aggregator collects and merges values from AddonValue resources.
// It implements priority-based deep merge where higher priority values override lower ones.
type Aggregator struct {
	client client.Client
}

// NewAggregator creates a new Aggregator instance.
func NewAggregator(c client.Client) *Aggregator {
	return &Aggregator{client: c}
}

// AggregateValues collects and merges values from matching AddonValues.
//
// Algorithm:
// 1. Collect selectors from spec.valuesSelectors + status.phaseValuesSelector
// 2. Sort selectors by priority (ascending - lower first)
// 3. For each selector: find AddonValues matching labels
// 4. Sort matched AddonValues by name (stability)
// 5. Deep merge values sequentially (higher priority wins)
func (a *Aggregator) AggregateValues(ctx context.Context, addon *addonsv1alpha1.Addon) (map[string]any, error) {
	// Collect all selectors: static from spec + dynamic from status
	allSelectors := collectSelectors(addon)

	// Sort by priority (ascending - lower priority first, higher overrides)
	sort.SliceStable(allSelectors, func(i, j int) bool {
		return allSelectors[i].Priority < allSelectors[j].Priority
	})

	result := make(map[string]any)

	// Process each selector in priority order
	for _, selector := range allSelectors {
		values, err := a.aggregateForSelector(ctx, selector)
		if err != nil {
			return nil, err
		}
		result = DeepMerge(result, values)
	}

	return result, nil
}

// collectSelectors combines spec and status selectors.
func collectSelectors(addon *addonsv1alpha1.Addon) []addonsv1alpha1.ValuesSelector {
	result := make([]addonsv1alpha1.ValuesSelector, 0,
		len(addon.Spec.ValuesSelectors)+len(addon.Status.PhaseValuesSelector))

	result = append(result, addon.Spec.ValuesSelectors...)
	result = append(result, addon.Status.PhaseValuesSelector...)

	return result
}

// aggregateForSelector finds and merges all AddonValues matching the selector.
// Uses exact match for addon labels (only labels with addons.in-cloud.io/ prefix are compared).
// Optimization: Uses server-side MatchingLabels as pre-filter, then exact match on the smaller set.
func (a *Aggregator) aggregateForSelector(ctx context.Context, selector addonsv1alpha1.ValuesSelector) (map[string]any, error) {
	// Extract addon-prefixed labels for server-side pre-filtering
	addonLabels := FilterAddonLabels(selector.MatchLabels)

	var prefilteredValues addonsv1alpha1.AddonValueList
	listOpts := []client.ListOption{}
	if len(addonLabels) > 0 {
		// Use MatchingLabels for server-side filtering (subset match)
		listOpts = append(listOpts, client.MatchingLabels(addonLabels))
	}

	if err := a.client.List(ctx, &prefilteredValues, listOpts...); err != nil {
		return nil, fmt.Errorf("list AddonValues for selector %s: %w", selector.Name, err)
	}

	// Post-filter for exact match (required because MatchingLabels does subset match)
	var matchingValues []addonsv1alpha1.AddonValue
	for _, av := range prefilteredValues.Items {
		if ExactMatchAddonLabels(selector.MatchLabels, av.Labels) {
			matchingValues = append(matchingValues, av)
		}
	}

	// Sort by name for deterministic ordering
	sort.Slice(matchingValues, func(i, j int) bool {
		return matchingValues[i].Name < matchingValues[j].Name
	})

	result := make(map[string]any)

	// Merge values from each matching AddonValue
	for _, av := range matchingValues {
		values, err := UnmarshalValues(av.Spec.Values.Raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal values from %s: %w", av.Name, err)
		}
		result = DeepMerge(result, values)
	}

	return result, nil
}

// UnmarshalValues converts runtime.RawExtension.Raw to map[string]any.
func UnmarshalValues(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return make(map[string]any), nil
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	return result, nil
}

// UnmarshalRawExtension converts runtime.RawExtension to map[string]any.
func UnmarshalRawExtension(ext []byte) (map[string]any, error) {
	return UnmarshalValues(ext)
}

// DeepMerge performs strategic merge of two maps.
// Merge strategy:
// - Objects (maps): recursive merge
// - Arrays: replace (overlay wins)
// - Scalars: replace (overlay wins)
func DeepMerge(base, overlay map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	if overlay == nil {
		return base
	}

	result := make(map[string]any)

	// Copy base values
	for k, v := range base {
		result[k] = v
	}

	// Merge overlay values
	for k, overlayVal := range overlay {
		baseVal, exists := result[k]
		if !exists {
			result[k] = deepCopy(overlayVal)
			continue
		}

		// If both are maps, merge recursively
		baseMap, baseIsMap := baseVal.(map[string]any)
		overlayMap, overlayIsMap := overlayVal.(map[string]any)

		if baseIsMap && overlayIsMap {
			result[k] = DeepMerge(baseMap, overlayMap)
		} else {
			// For arrays and scalars, overlay wins
			result[k] = deepCopy(overlayVal)
		}
	}

	return result
}

// deepCopy creates a deep copy of a value.
func deepCopy(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v := range val {
			result[k] = deepCopy(v)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, v := range val {
			result[i] = deepCopy(v)
		}
		return result
	default:
		// Scalars are immutable, return as-is
		return val
	}
}

// ExactMatchAddonLabels checks if selector labels match resource labels EXACTLY.
// Only compares labels with prefix "addons.in-cloud.io/".
// All other labels (kubernetes.io/*, app.kubernetes.io/*, etc.) are ignored.
func ExactMatchAddonLabels(selector, resource map[string]string) bool {
	selectorAddon := FilterAddonLabels(selector)
	resourceAddon := FilterAddonLabels(resource)

	// Exact match requires same count of addon labels
	if len(selectorAddon) != len(resourceAddon) {
		return false
	}

	// Check each selector label exists in resource with same value
	for k, v := range selectorAddon {
		if resourceAddon[k] != v {
			return false
		}
	}
	return true
}

// FilterAddonLabels keeps ONLY labels with prefix "addons.in-cloud.io/".
func FilterAddonLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range labels {
		if strings.HasPrefix(k, AddonLabelPrefix) {
			result[k] = v
		}
	}
	return result
}

// ComputeHash generates a hash of the values for change detection.
func ComputeHash(values map[string]any) (string, error) {
	// Use canonical JSON for deterministic ordering
	data, err := canonicalJSON(values)
	if err != nil {
		return "", fmt.Errorf("marshal values: %w", err)
	}

	// Simple hash using FNV-1a
	var hash uint64 = 14695981039346656037
	for _, b := range data {
		hash ^= uint64(b)
		hash *= 1099511628211
	}

	return fmt.Sprintf("%016x", hash), nil
}

// canonicalJSON produces deterministic JSON with sorted keys.
func canonicalJSON(v any) ([]byte, error) {
	return json.Marshal(sortedMap(v))
}

// sortedMap recursively converts maps to ensure deterministic iteration.
func sortedMap(v any) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v := range val {
			result[k] = sortedMap(v)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, v := range val {
			result[i] = sortedMap(v)
		}
		return result
	default:
		return val
	}
}
