//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
)

// Test timeouts and intervals
const (
	// timeout is the default timeout for Eventually assertions
	timeout = 2 * time.Minute
	// longTimeout is used for operations that may take longer (e.g., Argo CD sync)
	longTimeout = 5 * time.Minute
	// interval is the polling interval for Eventually assertions
	interval = 1 * time.Second
)

// testNamespace is used for namespaced resources (Secrets, etc.).
const testNamespace = "default"

// ctx is the context used for all k8s client operations
var ctx = context.Background()

// k8sClient is the controller-runtime client for k8s API operations
var k8sClient client.Client

// initK8sClient initializes the k8s client using the current kubeconfig.
// This should be called in BeforeSuite or BeforeAll of tests that need k8s access.
func initK8sClient() error {
	// Load kubeconfig from default location
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Register our CRD types
	if err := addonsv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return fmt.Errorf("failed to add addon types to scheme: %w", err)
	}

	// Register Argo CD types
	if err := argocdv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return fmt.Errorf("failed to add argocd types to scheme: %w", err)
	}

	// Create a dynamic RESTMapper that refreshes on cache misses.
	// This is important because CRDs are installed during BeforeSuite,
	// and a cached mapper might not have discovered them yet.
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %w", err)
	}
	mapper, err := apiutil.NewDynamicRESTMapper(config, httpClient)
	if err != nil {
		return fmt.Errorf("failed to create dynamic RESTMapper: %w", err)
	}

	// Create the client with the dynamic mapper
	k8sClient, err = client.New(config, client.Options{
		Scheme: scheme.Scheme,
		Mapper: mapper,
	})
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	return nil
}

// uniqueName generates a unique resource name for test isolation.
// Format: prefix-<8 random chars>
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.NewString()[:8])
}

// waitForDeletion waits for a resource to be deleted from the cluster.
func waitForDeletion(obj client.Object) {
	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, obj)
		return errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue(), "Resource should be deleted: %s/%s", key.Namespace, key.Name)
}

// cleanupResource deletes a resource and waits for it to be gone.
// Safe to call even if resource doesn't exist.
func cleanupResource(obj client.Object) {
	err := k8sClient.Delete(ctx, obj)
	if err != nil && !errors.IsNotFound(err) {
		return
	}
	if !errors.IsNotFound(err) {
		waitForDeletion(obj)
	}
}

// createTestAddonValue creates an AddonValue for testing.
func createTestAddonValue(name, addonName string, values map[string]interface{}, extraLabels map[string]string) *addonsv1alpha1.AddonValue {
	labels := map[string]string{
		"addons.in-cloud.io/addon": addonName,
	}
	for k, v := range extraLabels {
		labels[k] = v
	}

	av := &addonsv1alpha1.AddonValue{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: addonsv1alpha1.AddonValueSpec{},
	}

	// Set values if provided — marshal map to YAML string
	if values != nil {
		av.Spec.Values = string(mustMarshalYAML(values))
	}

	Expect(k8sClient.Create(ctx, av)).To(Succeed(), "Failed to create AddonValue %s", name)
	return av
}

// AddonOption is a functional option for createTestAddon
type AddonOption func(*addonsv1alpha1.Addon)

// WithValuesSelector adds a values selector to the Addon
func WithValuesSelector(name string, labels map[string]string, priority int) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.ValuesSelectors = append(a.Spec.ValuesSelectors, addonsv1alpha1.ValuesSelector{
			Name:        name,
			Priority:    priority,
			MatchLabels: labels,
		})
	}
}

// WithTargetNamespace sets the target namespace for the Addon
func WithTargetNamespace(ns string) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.TargetNamespace = ns
	}
}

// WithVariables sets variables for template rendering
func WithVariables(vars map[string]string) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.Variables = vars
	}
}

// WithoutAutoSync disables auto-sync for the Addon.
// Use this for tests that need the Addon to stay in a non-Ready state.
func WithoutAutoSync() AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.Backend.SyncPolicy = nil
	}
}

// createTestAddon creates an Addon for testing with optional modifiers.
// Uses the lightweight podinfo chart for fast sync in tests.
func createTestAddon(name string, opts ...AddonOption) *addonsv1alpha1.Addon {
	addon := &addonsv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: addonsv1alpha1.AddonSpec{
			// podinfo is a lightweight chart specifically designed for testing
			// https://github.com/stefanprodan/podinfo
			Chart:           "podinfo",
			RepoURL:         "https://stefanprodan.github.io/podinfo",
			Version:         "6.5.0",
			TargetCluster:   "in-cluster",
			TargetNamespace: testNamespace,
			Backend: addonsv1alpha1.BackendSpec{
				Type:      "argocd",
				Namespace: "argocd",
				// Enable auto-sync for e2e tests to verify full reconciliation cycle
				SyncPolicy: &addonsv1alpha1.SyncPolicy{
					Automated: &addonsv1alpha1.AutomatedSync{
						Prune:    true,
						SelfHeal: true,
					},
				},
			},
		},
	}

	for _, opt := range opts {
		opt(addon)
	}

	Expect(k8sClient.Create(ctx, addon)).To(Succeed(), "Failed to create Addon %s", name)
	return addon
}

// getAddon retrieves an Addon by name.
func getAddon(name string) (*addonsv1alpha1.Addon, error) {
	addon := &addonsv1alpha1.Addon{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, addon)
	return addon, err
}

// waitForAddonReady waits for an Addon to have Ready condition True.
// This replaces the deprecated waitForAddonPhase function.
func waitForAddonReady(name string) {
	waitForCondition(name, "Ready", metav1.ConditionTrue)
}

// waitForCondition waits for an Addon condition to have the specified status.
func waitForCondition(name, condType string, status metav1.ConditionStatus) {
	Eventually(func() metav1.ConditionStatus {
		addon, err := getAddon(name)
		if err != nil {
			return metav1.ConditionUnknown
		}
		for _, c := range addon.Status.Conditions {
			if c.Type == condType {
				return c.Status
			}
		}
		return metav1.ConditionUnknown
	}, timeout, interval).Should(Equal(status), "Addon %s condition %s should be %s", name, condType, status)
}

// =============================================================================
// Argo CD Application Helpers
// =============================================================================

// argoCDNamespace is the namespace where Argo CD Applications are created
const argoCDNamespace = "argocd"

// waitForApplication waits for an Application to exist in the cluster.
func waitForApplication(name, namespace string) *argocdv1alpha1.Application {
	var app *argocdv1alpha1.Application
	Eventually(func() error {
		app = &argocdv1alpha1.Application{}
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, app)
	}, timeout, interval).Should(Succeed(), "Application %s/%s should exist", namespace, name)
	return app
}

// waitForApplicationSynced waits for an Application to reach Synced status.
func waitForApplicationSynced(name, namespace string) *argocdv1alpha1.Application {
	var app *argocdv1alpha1.Application
	Eventually(func() argocdv1alpha1.SyncStatusCode {
		app = &argocdv1alpha1.Application{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, app); err != nil {
			return ""
		}
		if app.Status.Sync.Status == "" {
			return ""
		}
		return app.Status.Sync.Status
	}, longTimeout, interval).Should(Equal(argocdv1alpha1.SyncStatusCodeSynced),
		"Application %s/%s should be synced", namespace, name)
	return app
}

// waitForApplicationHealthy waits for an Application to reach Healthy status.
func waitForApplicationHealthy(name, namespace string) *argocdv1alpha1.Application {
	var app *argocdv1alpha1.Application
	Eventually(func() health.HealthStatusCode {
		app = &argocdv1alpha1.Application{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, app); err != nil {
			return ""
		}
		if app.Status.Health.Status == "" {
			return ""
		}
		return app.Status.Health.Status
	}, longTimeout, interval).Should(Equal(health.HealthStatusHealthy),
		"Application %s/%s should be healthy", namespace, name)
	return app
}

// waitForApplicationDeleted waits for an Application to be deleted from the cluster.
func waitForApplicationDeleted(name, namespace string) {
	Eventually(func() bool {
		app := &argocdv1alpha1.Application{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, app)
		return errors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue(), "Application %s/%s should be deleted", namespace, name)
}

// getApplication retrieves an Application by name and namespace.
func getApplication(name, namespace string) (*argocdv1alpha1.Application, error) {
	app := &argocdv1alpha1.Application{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, app)
	return app, err
}

// getApplicationValues extracts Helm values from an Application spec.
// Returns an empty map if no values are set.
func getApplicationValues(app *argocdv1alpha1.Application) map[string]interface{} {
	values := make(map[string]interface{})

	if app.Spec.Source == nil {
		return values
	}

	if app.Spec.Source.Helm == nil {
		return values
	}

	if app.Spec.Source.Helm.Values == "" {
		return values
	}

	if err := yaml.Unmarshal([]byte(app.Spec.Source.Helm.Values), &values); err != nil {
		return values
	}

	return values
}

// getApplicationSyncStatus returns the current sync status of an Application.
func getApplicationSyncStatus(name, namespace string) argocdv1alpha1.SyncStatusCode {
	app, err := getApplication(name, namespace)
	if err != nil {
		return ""
	}
	return app.Status.Sync.Status
}

// getApplicationHealthStatus returns the current health status of an Application.
func getApplicationHealthStatus(name, namespace string) health.HealthStatusCode {
	app, err := getApplication(name, namespace)
	if err != nil {
		return ""
	}
	return app.Status.Health.Status
}

// =============================================================================
// Additional Addon Helpers
// =============================================================================

// WithValueSource adds a value source to the Addon for extracting values from external resources.
func WithValueSource(name, apiVersion, kind, resourceName, namespace string, extract ExtractRule) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.ValuesSources = append(a.Spec.ValuesSources, addonsv1alpha1.ValueSource{
			Name: name,
			SourceRef: addonsv1alpha1.SourceRef{
				APIVersion: apiVersion,
				Kind:       kind,
				Name:       resourceName,
				Namespace:  namespace,
			},
			Extract: []addonsv1alpha1.ExtractRule{{
				JSONPath: extract.JSONPath,
				As:       extract.As,
				Decode:   extract.Decode,
			}},
		})
	}
}

// ExtractRule defines how to extract a value from a resource.
type ExtractRule struct {
	JSONPath string
	As       string
	Decode   string
}

// WithInitDependencies adds an init dependency to the Addon.
func WithInitDependencies(name string, criteria ...addonsv1alpha1.Criterion) AddonOption {
	return func(a *addonsv1alpha1.Addon) {
		a.Spec.InitDependencies = append(a.Spec.InitDependencies, addonsv1alpha1.Dependency{
			Name:     name,
			Criteria: criteria,
		})
	}
}

// waitForConditionReason waits for an Addon condition to have the specified reason.
func waitForConditionReason(name, condType, reason string) {
	Eventually(func() string {
		addon, err := getAddon(name)
		if err != nil {
			return ""
		}
		for _, c := range addon.Status.Conditions {
			if c.Type == condType {
				return c.Reason
			}
		}
		return ""
	}, timeout, interval).Should(Equal(reason), "Addon %s condition %s should have reason %s", name, condType, reason)
}

// createTestSecret creates a Secret for testing valuesSources.
func createTestSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed(), "Failed to create Secret %s/%s", namespace, name)
	return secret
}

// deleteTestSecret deletes a Secret.
func deleteTestSecret(name, namespace string) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	_ = k8sClient.Delete(ctx, secret)
}

// =============================================================================
// Utility Functions
// =============================================================================

// mustMarshal marshals the given value to JSON and panics on error.
// Use this only in tests where marshaling should never fail.
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshal failed: %v", err))
	}
	return data
}

// mustMarshalYAML marshals the given value to YAML and panics on error.
func mustMarshalYAML(v interface{}) []byte {
	data, err := yaml.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshalYAML failed: %v", err))
	}
	return data
}

// =============================================================================
// AddonPhase Helpers
// =============================================================================

// jsonValue creates an apiextensionsv1.JSON from any value.
// Used for Criterion.Value field which requires JSON encoding.
func jsonValue(v interface{}) *apiextensionsv1.JSON {
	data := mustMarshal(v)
	return &apiextensionsv1.JSON{Raw: data}
}

// AddonPhaseOption is a functional option for createTestAddonPhase
type AddonPhaseOption func(*addonsv1alpha1.AddonPhase)

// WithPhaseRule adds a rule to the AddonPhase.
func WithPhaseRule(name string, criteria []addonsv1alpha1.Criterion, selector addonsv1alpha1.ValuesSelector) AddonPhaseOption {
	return func(ap *addonsv1alpha1.AddonPhase) {
		ap.Spec.Rules = append(ap.Spec.Rules, addonsv1alpha1.PhaseRule{
			Name:     name,
			Criteria: criteria,
			Selector: selector,
		})
	}
}

// WithAlwaysActiveRule adds a rule with empty criteria (always matches).
// Useful for testing phaseValuesSelector injection without dependencies.
func WithAlwaysActiveRule(addonName, ruleName string, priority int) AddonPhaseOption {
	return func(ap *addonsv1alpha1.AddonPhase) {
		ap.Spec.Rules = append(ap.Spec.Rules, addonsv1alpha1.PhaseRule{
			Name:     ruleName,
			Criteria: []addonsv1alpha1.Criterion{}, // Empty = always match
			Selector: addonsv1alpha1.ValuesSelector{
				Name:     ruleName,
				Priority: priority,
				MatchLabels: map[string]string{
					"addons.in-cloud.io/addon": addonName,
					"always-active":            "true",
				},
			},
		})
	}
}

// createTestAddonPhase creates an AddonPhase for testing with optional modifiers.
// The name should match the target Addon name (1:1 relationship).
func createTestAddonPhase(name string, opts ...AddonPhaseOption) *addonsv1alpha1.AddonPhase {
	phase := &addonsv1alpha1.AddonPhase{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: addonsv1alpha1.AddonPhaseSpec{
			Rules: []addonsv1alpha1.PhaseRule{},
		},
	}

	for _, opt := range opts {
		opt(phase)
	}

	Expect(k8sClient.Create(ctx, phase)).To(Succeed(), "Failed to create AddonPhase %s", name)
	return phase
}

// getAddonPhase retrieves an AddonPhase by name.
func getAddonPhase(name string) (*addonsv1alpha1.AddonPhase, error) {
	phase := &addonsv1alpha1.AddonPhase{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, phase)
	return phase, err
}

// waitForPhaseRuleMatched waits for an AddonPhase rule to have the specified matched status.
func waitForPhaseRuleMatched(name, ruleName string, matched bool) {
	Eventually(func() bool {
		phase, err := getAddonPhase(name)
		if err != nil {
			return false
		}
		for _, rs := range phase.Status.RuleStatuses {
			if rs.Name == ruleName {
				return rs.Matched == matched
			}
		}
		return false // Rule status not found yet
	}, timeout, interval).Should(BeTrue(),
		"AddonPhase %s rule %s should have matched=%v", name, ruleName, matched)
}

// waitForPhaseValuesSelector waits for Addon to have phase-injected selectors.
func waitForPhaseValuesSelector(addonName string, minCount int) {
	Eventually(func() int {
		addon, err := getAddon(addonName)
		if err != nil {
			return 0
		}
		return len(addon.Status.PhaseValuesSelector)
	}, timeout, interval).Should(BeNumerically(">=", minCount),
		"Addon %s should have at least %d phaseValuesSelector entries", addonName, minCount)
}

// CriterionBuilder helps construct Criterion objects for tests.
type CriterionBuilder struct {
	criterion addonsv1alpha1.Criterion
}

// NewCriterion creates a new CriterionBuilder.
func NewCriterion() *CriterionBuilder {
	return &CriterionBuilder{}
}

// WithSource sets the source for the criterion.
func (b *CriterionBuilder) WithSource(apiVersion, kind, name, namespace string) *CriterionBuilder {
	b.criterion.Source = &addonsv1alpha1.CriterionSource{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespace,
	}
	return b
}

// WithJSONPath sets the JSONPath for the criterion.
func (b *CriterionBuilder) WithJSONPath(path string) *CriterionBuilder {
	b.criterion.JSONPath = path
	return b
}

// WithOperator sets the operator for the criterion.
func (b *CriterionBuilder) WithOperator(op addonsv1alpha1.CriterionOperator) *CriterionBuilder {
	b.criterion.Operator = op
	return b
}

// WithValue sets the value for comparison.
func (b *CriterionBuilder) WithValue(v interface{}) *CriterionBuilder {
	b.criterion.Value = jsonValue(v)
	return b
}

// WithKeep sets the keep (rule latching) flag for the criterion.
func (b *CriterionBuilder) WithKeep(keep bool) *CriterionBuilder {
	b.criterion.Keep = &keep
	return b
}

// Build returns the constructed Criterion.
func (b *CriterionBuilder) Build() addonsv1alpha1.Criterion {
	return b.criterion
}

// =============================================================================
// AddonClaim / AddonTemplate Helpers
// =============================================================================

// createTestAddonTemplate creates a cluster-scoped AddonTemplate for testing.
func createTestAddonTemplate(name, templateStr string) *addonsv1alpha1.AddonTemplate {
	tmpl := &addonsv1alpha1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: addonsv1alpha1.AddonTemplateSpec{
			Template: templateStr,
		},
	}
	Expect(k8sClient.Create(ctx, tmpl)).To(Succeed(), "Failed to create AddonTemplate %s", name)
	return tmpl
}

// AddonClaimOption is a functional option for createTestAddonClaim.
type AddonClaimOption func(*addonsv1alpha1.AddonClaim)

// WithClaimAnnotations sets annotations on the AddonClaim.
func WithClaimAnnotations(annotations map[string]string) AddonClaimOption {
	return func(c *addonsv1alpha1.AddonClaim) {
		if c.Annotations == nil {
			c.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			c.Annotations[k] = v
		}
	}
}

// WithClaimVariables sets variables as JSON on the AddonClaim.
func WithClaimVariables(vars map[string]string) AddonClaimOption {
	return func(c *addonsv1alpha1.AddonClaim) {
		data := mustMarshal(vars)
		c.Spec.Variables = &apiextensionsv1.JSON{Raw: data}
	}
}

// WithClaimValuesString sets the valuesString field on the AddonClaim.
func WithClaimValuesString(values string) AddonClaimOption {
	return func(c *addonsv1alpha1.AddonClaim) {
		c.Spec.ValuesString = values
	}
}

// WithClaimValueLabels sets the valueLabels field on the AddonClaim.
func WithClaimValueLabels(label string) AddonClaimOption {
	return func(c *addonsv1alpha1.AddonClaim) {
		c.Spec.ValueLabels = label
	}
}

// WithClaimAddon sets the addon identity on the AddonClaim.
func WithClaimAddon(name string) AddonClaimOption {
	return func(c *addonsv1alpha1.AddonClaim) {
		c.Spec.Addon = addonsv1alpha1.AddonIdentity{Name: name}
	}
}

// createTestAddonClaim creates a namespaced AddonClaim for testing.
func createTestAddonClaim(name, namespace, templateRef, credentialRef string, opts ...AddonClaimOption) *addonsv1alpha1.AddonClaim {
	claim := &addonsv1alpha1.AddonClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: addonsv1alpha1.AddonClaimSpec{
			TemplateRef:   addonsv1alpha1.TemplateRef{Name: templateRef},
			CredentialRef: addonsv1alpha1.CredentialRef{Name: credentialRef},
		},
	}

	for _, opt := range opts {
		opt(claim)
	}

	Expect(k8sClient.Create(ctx, claim)).To(Succeed(), "Failed to create AddonClaim %s/%s", namespace, name)
	return claim
}

// getAddonClaim retrieves an AddonClaim by name and namespace.
func getAddonClaim(name, namespace string) (*addonsv1alpha1.AddonClaim, error) {
	claim := &addonsv1alpha1.AddonClaim{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, claim)
	return claim, err
}

// waitForClaimCondition waits for an AddonClaim condition to have the specified status.
func waitForClaimCondition(name, namespace, condType string, status metav1.ConditionStatus) {
	Eventually(func() metav1.ConditionStatus {
		claim, err := getAddonClaim(name, namespace)
		if err != nil {
			return metav1.ConditionUnknown
		}
		for _, c := range claim.Status.Conditions {
			if c.Type == condType {
				return c.Status
			}
		}
		return metav1.ConditionUnknown
	}, timeout, interval).Should(Equal(status),
		"AddonClaim %s/%s condition %s should be %s", namespace, name, condType, status)
}

// createKubeconfigSecret creates a Secret containing the current cluster's kubeconfig.
// This produces a self-referencing kubeconfig (the "remote" cluster is the same Kind cluster).
func createKubeconfigSecret(name, namespace string) *corev1.Secret {
	// Get kubeconfig from the Kind cluster
	kindCluster := os.Getenv("KIND_CLUSTER")
	if kindCluster == "" {
		kindCluster = "addon-operator-test-e2e"
	}
	kindBinary := os.Getenv("KIND")
	if kindBinary == "" {
		kindBinary = "kind"
	}

	cmd := exec.Command(kindBinary, "get", "kubeconfig", "--name", kindCluster, "--internal")
	kubeconfig, err := cmd.Output()
	Expect(err).NotTo(HaveOccurred(), "Failed to get kubeconfig from Kind cluster")

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"value": kubeconfig,
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed(), "Failed to create kubeconfig Secret %s/%s", namespace, name)
	return secret
}
