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

import (
	"fmt"
	"strings"

	rfc9535 "github.com/theory/jsonpath"
)

// Parse validates an RFC 9535 JSONPath expression.
// Path must start with "$" (e.g., "$.status.phase").
// Returns error if the path is syntactically invalid.
func Parse(path string) error {
	if path == "" || path == "$" {
		return nil
	}
	if !strings.HasPrefix(path, "$") {
		return fmt.Errorf("path must start with '$': %q", path)
	}
	_, err := rfc9535.Parse(path)

	return err
}
