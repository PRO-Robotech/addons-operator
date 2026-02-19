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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name: "empty path",
			path: "",
		},
		{
			name: "root dollar",
			path: "$",
		},
		{
			name: "simple field",
			path: "$.status.phase",
		},
		{
			name: "nested fields",
			path: "$.metadata.labels.app",
		},
		{
			name: "array index",
			path: "$.status.conditions[0].status",
		},
		{
			name: "bracket notation for dotted key",
			path: "$.metadata.labels['app.kubernetes.io/name']",
		},
		{
			name: "filter expression",
			path: "$.status.conditions[?@.type=='Ready'].status",
		},
		{
			name: "deep nesting",
			path: "$.a.b.c.d.e",
		},
		{
			name:    "path without dollar prefix",
			path:    "status.phase",
			wantErr: true,
		},
		{
			name:    "old kubectl dot prefix",
			path:    ".status.phase",
			wantErr: true,
		},
		{
			name:    "invalid expression",
			path:    "$[invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Parse(tt.path)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestExtractString(t *testing.T) {
	obj := map[string]any{
		"status": map[string]any{
			"phase": "Running",
			"conditions": []any{
				map[string]any{"type": "Initialized", "status": "False"},
				map[string]any{"type": "Ready", "status": "True"},
				map[string]any{"type": "Available", "status": "True"},
			},
		},
		"metadata": map[string]any{
			"name": "test-pod",
			"labels": map[string]any{
				"app":                    "myapp",
				"app.kubernetes.io/name": "myapp",
			},
		},
		"data": map[string]any{
			"enable_hubble": "true",
		},
	}

	tests := []struct {
		name      string
		path      string
		wantVal   any
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "simple field",
			path:      "$.status.phase",
			wantVal:   "Running",
			wantFound: true,
		},
		{
			name:      "array index",
			path:      "$.status.conditions[0].status",
			wantVal:   "False",
			wantFound: true,
		},
		{
			name:      "filter expression",
			path:      "$.status.conditions[?@.type=='Ready'].status",
			wantVal:   "True",
			wantFound: true,
		},
		{
			name:      "bracket notation for dotted key",
			path:      "$.metadata.labels['app.kubernetes.io/name']",
			wantVal:   "myapp",
			wantFound: true,
		},
		{
			name:      "data field with underscore",
			path:      "$.data.enable_hubble",
			wantVal:   "true",
			wantFound: true,
		},
		{
			name:      "nonexistent path",
			path:      "$.nonexistent.path",
			wantFound: false,
		},
		{
			name:      "empty path returns root",
			path:      "",
			wantVal:   obj,
			wantFound: true,
		},
		{
			name:      "dollar path returns root",
			path:      "$",
			wantVal:   obj,
			wantFound: true,
		},
		{
			name:    "invalid path without dollar prefix",
			path:    "status/phase",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found, err := ExtractString(obj, tt.path)
			if tt.wantErr {
				assert.Error(t, err)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.wantVal, val)
			}
		})
	}
}
