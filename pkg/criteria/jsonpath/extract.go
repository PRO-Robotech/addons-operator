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

package jsonpath

// Extract extracts a value from an object using the parsed path segments.
// Returns (value, found, error).
// If the path is not found, returns (nil, false, nil) - not an error.
func Extract(obj any, segments []PathSegment) (any, bool, error) {
	if len(segments) == 0 {
		return obj, true, nil
	}

	current := obj
	for _, seg := range segments {
		val, found := seg.Extract(current)
		if !found {
			return nil, false, nil
		}
		current = val
	}

	return current, true, nil
}

// ExtractString is a convenience function that parses the path and extracts the value.
func ExtractString(obj any, path string) (any, bool, error) {
	segments, err := Parse(path)
	if err != nil {
		return nil, false, err
	}
	return Extract(obj, segments)
}
