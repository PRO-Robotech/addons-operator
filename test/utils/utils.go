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

package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	certmanagerVersion = "v1.19.1"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"

	// ArgoCDVersion is the version of Argo CD to install for e2e tests
	ArgoCDVersion   = "v2.14.0"
	argoCDURLTmpl   = "https://raw.githubusercontent.com/argoproj/argo-cd/%s/manifests/install.yaml"
	argoCDNamespace = "argocd"

	defaultKindBinary  = "kind"
	defaultKindCluster = "addon-operator-test-e2e"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// UninstallCertManager uninstalls the cert manager.
// Uses timeouts to prevent hanging.
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	cmd := exec.CommandContext(ctx, "kubectl", "delete", "-f", url, "--ignore-not-found", "--wait=false")
	if _, err := Run(cmd); err != nil {
		if ctx.Err() != context.DeadlineExceeded {
			warnError(err)
		}
	}
	cancel()

	// Delete leftover leases in kube-system (not cleaned by default)
	kubeSystemLeases := []string{
		"cert-manager-cainjector-leader-election",
		"cert-manager-controller",
	}
	for _, lease := range kubeSystemLeases {
		leaseCtx, leaseCancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd = exec.CommandContext(leaseCtx, "kubectl", "delete", "lease", lease,
			"-n", "kube-system", "--ignore-not-found", "--force", "--grace-period=0")
		if _, err := Run(cmd); err != nil {
			if leaseCtx.Err() != context.DeadlineExceeded {
				warnError(err)
			}
		}
		leaseCancel()
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	if _, err := Run(cmd); err != nil {
		return err
	}

	// Wait for cert-manager webhook to be fully ready to accept requests.
	// The deployment being Available doesn't mean the webhook's CA certificate
	// has been propagated yet. We need to wait until it can validate certificates.
	return waitForCertManagerWebhookReady()
}

// waitForCertManagerWebhookReady waits until cert-manager webhook is fully ready
// by attempting to create a test ClusterIssuer. This ensures the webhook's CA
// certificate has been propagated.
func waitForCertManagerWebhookReady() error {
	testIssuerYAML := `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: test-selfsigned-issuer
spec:
  selfSigned: {}`

	// Retry for up to 60 seconds
	for i := range 30 {
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(testIssuerYAML)
		_, err := Run(cmd)
		if err == nil {
			// Success - clean up test issuer
			cmd = exec.Command("kubectl", "delete", "clusterissuer", "test-selfsigned-issuer", "--ignore-not-found")
			_, _ = Run(cmd)
			return nil
		}

		// Wait 2 seconds before retry
		_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for cert-manager webhook to be ready (attempt %d/30)...\n", i+1)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("cert-manager webhook not ready after 60 seconds")
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	cluster := defaultKindCluster
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	kindBinary := defaultKindBinary
	if v, ok := os.LookupEnv("KIND"); ok {
		kindBinary = v
	}
	cmd := exec.Command(kindBinary, kindOptions...)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// InstallArgoCD installs Argo CD into the cluster.
func InstallArgoCD() error {
	// Create argocd namespace (idempotent)
	cmd := exec.Command("kubectl", "create", "namespace", argoCDNamespace,
		"--dry-run=client", "-o", "yaml")
	nsYaml, err := Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to generate namespace yaml: %w", err)
	}

	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(nsYaml)
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to create argocd namespace: %w", err)
	}

	// Install Argo CD manifests
	url := fmt.Sprintf(argoCDURLTmpl, ArgoCDVersion)
	cmd = exec.Command("kubectl", "apply", "-n", argoCDNamespace, "-f", url)
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to install argocd: %w", err)
	}

	// Wait for all deployments to be available
	cmd = exec.Command("kubectl", "wait", "--for=condition=available",
		"deployment", "-n", argoCDNamespace, "--all", "--timeout=300s")
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("failed waiting for argocd deployments: %w", err)
	}

	return nil
}

// IsArgoCDInstalled checks if Argo CD CRDs are installed.
func IsArgoCDInstalled() bool {
	cmd := exec.Command("kubectl", "get", "crd", "applications.argoproj.io")
	_, err := Run(cmd)
	return err == nil
}

// UninstallArgoCD uninstalls Argo CD from the cluster.
// Uses timeouts to prevent hanging.
func UninstallArgoCD() {
	url := fmt.Sprintf(argoCDURLTmpl, ArgoCDVersion)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	cmd := exec.CommandContext(
		ctx, "kubectl", "delete", "-n", argoCDNamespace, "-f", url,
		"--ignore-not-found", "--wait=false",
	)
	if _, err := Run(cmd); err != nil {
		if ctx.Err() != context.DeadlineExceeded {
			warnError(err)
		}
	}
	cancel()

	// Delete namespace
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	cmd = exec.CommandContext(ctx, "kubectl", "delete", "namespace", argoCDNamespace, "--ignore-not-found", "--wait=false")
	if _, err := Run(cmd); err != nil {
		if ctx.Err() != context.DeadlineExceeded {
			warnError(err)
		}
	}
	cancel()
}

// InstallCRDs installs the CRDs into the cluster using make install.
func InstallCRDs() error {
	cmd := exec.Command("make", "install")
	_, err := Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to install CRDs: %w", err)
	}
	return nil
}

// UninstallCRDs uninstalls the CRDs from the cluster using make uninstall.
// Uses a timeout to prevent hanging on stuck CRDs.
func UninstallCRDs() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "make", "uninstall")
	if _, err := Run(cmd); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: uninstall CRDs timed out after 60s, continuing...\n")
		} else {
			warnError(err)
		}
	}
}

// DeployOperator deploys the operator to the cluster using make deploy.
func DeployOperator(image string) error {
	cmd := exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", image))
	_, err := Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to deploy operator: %w", err)
	}

	// Wait for controller-manager to be ready
	cmd = exec.Command("kubectl", "wait", "--for=condition=available",
		"deployment/addon-operator-controller-manager",
		"-n", "addon-operator-system",
		"--timeout=120s")
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("failed waiting for operator deployment: %w", err)
	}

	// Wait for webhook endpoints to be ready
	return waitForWebhookReady()
}

// waitForWebhookReady waits until the operator's webhook service is ready to accept requests.
func waitForWebhookReady() error {
	_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for webhook endpoints to be ready...\n")

	// Retry for up to 60 seconds
	for i := range 30 {
		cmd := exec.Command("kubectl", "get", "endpointslices.discovery.k8s.io",
			"-n", "addon-operator-system",
			"-l", "kubernetes.io/service-name=addon-operator-webhook-service",
			"-o", "jsonpath={range .items[*]}{range .endpoints[*]}{.addresses[*]}{end}{end}")
		output, err := Run(cmd)
		if err == nil && output != "" {
			_, _ = fmt.Fprintf(GinkgoWriter, "Webhook endpoints ready: %s\n", output)
			// Give the webhook a moment to fully initialize
			time.Sleep(2 * time.Second)
			return nil
		}

		_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for webhook endpoints (attempt %d/30)...\n", i+1)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("webhook endpoints not ready after 60 seconds")
}

// UndeployOperator removes the operator from the cluster using make undeploy.
// Uses a timeout to prevent hanging on stuck resources.
func UndeployOperator() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "make", "undeploy")
	if _, err := Run(cmd); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: undeploy timed out after 60s, continuing...\n")
		} else {
			warnError(err)
		}
	}
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %q to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		if _, err = out.WriteString(strings.TrimPrefix(scanner.Text(), prefix)); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err = out.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	if _, err = out.Write(content[idx+len(target):]); err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	// false positive
	// nolint:gosec
	if err = os.WriteFile(filename, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", filename, err)
	}

	return nil
}
