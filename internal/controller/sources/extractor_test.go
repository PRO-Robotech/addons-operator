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

package sources

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// Note on Secret.Data encoding:
// Kubernetes Secret.Data is map[string][]byte, storing raw bytes.
// When converted to Unstructured (JSON), []byte fields are automatically
// base64 encoded. So to extract values from secrets, we need decode: base64.

func TestExtractor_Extract(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	certData := "-----BEGIN CERTIFICATE-----\nMIIC..."

	// Note: Secret.Data stores raw bytes. When serialized to JSON (Unstructured),
	// the bytes are automatically base64 encoded. So we store raw bytes here.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cert-manager-ca",
			Namespace: "cert-manager",
		},
		Data: map[string][]byte{
			"ca.crt": []byte(certData),
		},
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "addon-config",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"logLevel": "debug",
			"endpoint": "https://api.example.com",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, configMap).
		Build()

	extractor := NewExtractor(client)

	t.Run("extract from Secret with base64 decode", func(t *testing.T) {
		sources := []addonsv1alpha1.ValueSource{
			{
				Name: "cert-manager-ca",
				SourceRef: addonsv1alpha1.SourceRef{
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       "cert-manager-ca",
					Namespace:  "cert-manager",
				},
				Extract: []addonsv1alpha1.ExtractRule{
					{
						JSONPath: ".data[\"ca.crt\"]",
						As:       "certs.caBundle",
						Decode:   "base64",
					},
				},
			},
		}

		result, err := extractor.Extract(context.Background(), sources)
		require.NoError(t, err)

		certs, ok := result["certs"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, certData, certs["caBundle"])
	})

	t.Run("extract from ConfigMap", func(t *testing.T) {
		sources := []addonsv1alpha1.ValueSource{
			{
				Name: "config",
				SourceRef: addonsv1alpha1.SourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "addon-config",
					Namespace:  "kube-system",
				},
				Extract: []addonsv1alpha1.ExtractRule{
					{
						JSONPath: ".data.logLevel",
						As:       "logging.level",
					},
					{
						JSONPath: ".data.endpoint",
						As:       "api.endpoint",
					},
				},
			},
		}

		result, err := extractor.Extract(context.Background(), sources)
		require.NoError(t, err)

		logging, ok := result["logging"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "debug", logging["level"])

		api, ok := result["api"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "https://api.example.com", api["endpoint"])
	})

	t.Run("multiple sources", func(t *testing.T) {
		sources := []addonsv1alpha1.ValueSource{
			{
				Name: "cert-manager-ca",
				SourceRef: addonsv1alpha1.SourceRef{
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       "cert-manager-ca",
					Namespace:  "cert-manager",
				},
				Extract: []addonsv1alpha1.ExtractRule{
					{
						JSONPath: ".data[\"ca.crt\"]",
						As:       "tls.ca",
						Decode:   "base64",
					},
				},
			},
			{
				Name: "config",
				SourceRef: addonsv1alpha1.SourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "addon-config",
					Namespace:  "kube-system",
				},
				Extract: []addonsv1alpha1.ExtractRule{
					{
						JSONPath: ".data.logLevel",
						As:       "config.logLevel",
					},
				},
			},
		}

		result, err := extractor.Extract(context.Background(), sources)
		require.NoError(t, err)

		// Check tls.ca
		tls, ok := result["tls"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, certData, tls["ca"])

		// Check config.logLevel
		config, ok := result["config"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "debug", config["logLevel"])
	})

	t.Run("empty sources", func(t *testing.T) {
		result, err := extractor.Extract(context.Background(), nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("source not found", func(t *testing.T) {
		sources := []addonsv1alpha1.ValueSource{
			{
				Name: "missing",
				SourceRef: addonsv1alpha1.SourceRef{
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       "nonexistent",
					Namespace:  "default",
				},
				Extract: []addonsv1alpha1.ExtractRule{
					{JSONPath: ".data.key", As: "key"},
				},
			},
		}

		_, err := extractor.Extract(context.Background(), sources)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source missing")
	})

	t.Run("path not found", func(t *testing.T) {
		sources := []addonsv1alpha1.ValueSource{
			{
				Name: "config",
				SourceRef: addonsv1alpha1.SourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "addon-config",
					Namespace:  "kube-system",
				},
				Extract: []addonsv1alpha1.ExtractRule{
					{
						JSONPath: ".data.nonexistent",
						As:       "value",
					},
				},
			},
		}

		_, err := extractor.Extract(context.Background(), sources)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("explicit namespace in SourceRef", func(t *testing.T) {
		// Create a secret in test-ns namespace
		testSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{
				"key": []byte("secretvalue"),
			},
		}

		clientWithNs := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(testSecret).
			Build()

		ext := NewExtractor(clientWithNs)

		sources := []addonsv1alpha1.ValueSource{
			{
				Name: "test-secret",
				SourceRef: addonsv1alpha1.SourceRef{
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       "test-secret",
					Namespace:  "test-ns", // Explicit namespace required
				},
				Extract: []addonsv1alpha1.ExtractRule{
					{JSONPath: ".data.key", As: "secret.key", Decode: "base64"},
				},
			},
		}

		result, err := ext.Extract(context.Background(), sources)
		require.NoError(t, err)

		secretVal, ok := result["secret"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "secretvalue", secretVal["key"])
	})
}

func TestExtractByPath(t *testing.T) {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      "test",
			"namespace": "default",
		},
		"data": map[string]any{
			"ca.crt":   "encoded-cert",
			"tls.key":  "encoded-key",
			"username": "admin",
		},
		"stringData": map[string]any{
			"config.yaml": "key: value",
		},
		"items": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}

	tests := []struct {
		name     string
		path     string
		expected any
		found    bool
		wantErr  bool
	}{
		{
			name:     "simple field",
			path:     ".kind",
			expected: "Secret",
			found:    true,
		},
		{
			name:     "nested field",
			path:     ".metadata.name",
			expected: "test",
			found:    true,
		},
		{
			name:     "data field",
			path:     ".data.username",
			expected: "admin",
			found:    true,
		},
		{
			name:     "bracket notation with dots",
			path:     `.data["ca.crt"]`,
			expected: "encoded-cert",
			found:    true,
		},
		{
			name:     "bracket notation single quotes",
			path:     `.data['tls.key']`,
			expected: "encoded-key",
			found:    true,
		},
		{
			name:     "stringData with dots",
			path:     `.stringData["config.yaml"]`,
			expected: "key: value",
			found:    true,
		},
		{
			name:     "array index",
			path:     ".items[0].name",
			expected: "first",
			found:    true,
		},
		{
			name:     "array second element",
			path:     ".items[1].name",
			expected: "second",
			found:    true,
		},
		{
			name:    "array index out of bounds",
			path:    ".items[99]",
			found:   false,
			wantErr: false,
		},
		{
			name:    "field not found",
			path:    ".nonexistent",
			found:   false,
			wantErr: false,
		},
		{
			name:    "nested field not found",
			path:    ".metadata.nonexistent",
			found:   false,
			wantErr: false,
		},
		{
			name:     "root path",
			path:     ".",
			expected: obj,
			found:    true,
		},
		{
			name:    "unclosed bracket",
			path:    ".data[invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found, err := extractByPath(obj, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.found, found)
			if tt.found {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDecodeValue(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		decode   string
		expected any
		wantErr  bool
	}{
		{
			name:     "no decode",
			value:    "plain",
			decode:   "",
			expected: "plain",
		},
		{
			name:     "base64 decode",
			value:    base64.StdEncoding.EncodeToString([]byte("decoded")),
			decode:   "base64",
			expected: "decoded",
		},
		{
			name:     "base64 decode multiline",
			value:    base64.StdEncoding.EncodeToString([]byte("line1\nline2\nline3")),
			decode:   "base64",
			expected: "line1\nline2\nline3",
		},
		{
			name:    "base64 non-string value",
			value:   123,
			decode:  "base64",
			wantErr: true,
		},
		{
			name:    "base64 invalid encoding",
			value:   "not-valid-base64!!!",
			decode:  "base64",
			wantErr: true,
		},
		{
			name:    "unknown decode method",
			value:   "value",
			decode:  "gzip",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeValue(tt.value, tt.decode)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetNestedField(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]any
		path     string
		value    any
		expected map[string]any
	}{
		{
			name:     "simple field",
			initial:  map[string]any{},
			path:     "key",
			value:    "value",
			expected: map[string]any{"key": "value"},
		},
		{
			name:    "nested field",
			initial: map[string]any{},
			path:    "a.b.c",
			value:   "value",
			expected: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": "value",
					},
				},
			},
		},
		{
			name:    "merge with existing",
			initial: map[string]any{"a": map[string]any{"x": "existing"}},
			path:    "a.y",
			value:   "new",
			expected: map[string]any{
				"a": map[string]any{
					"x": "existing",
					"y": "new",
				},
			},
		},
		{
			name:    "override non-map value",
			initial: map[string]any{"a": "string"},
			path:    "a.b",
			value:   "value",
			expected: map[string]any{
				"a": map[string]any{
					"b": "value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setNestedField(tt.initial, tt.path, tt.value)
			assert.Equal(t, tt.expected, tt.initial)
		})
	}
}

func TestSourceRefKey(t *testing.T) {
	ref := addonsv1alpha1.SourceRef{
		APIVersion: "v1",
		Kind:       "Secret",
		Namespace:  "cert-manager",
		Name:       "ca-secret",
	}

	key := SourceRefKey(ref)
	assert.Equal(t, "v1/Secret/cert-manager/ca-secret", key)
}
