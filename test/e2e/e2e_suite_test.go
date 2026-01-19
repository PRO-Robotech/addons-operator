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
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"addons-operator/test/utils"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// - ARGOCD_INSTALL_SKIP=true: Skips ArgoCD installation during test setup.
	// These variables are useful if CertManager/ArgoCD are already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	skipArgoCDInstall      = os.Getenv("ARGOCD_INSTALL_SKIP") == "true"

	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false
	// isArgoCDAlreadyInstalled will be set true when ArgoCD CRDs be found on the cluster
	isArgoCDAlreadyInstalled = false

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/addon-operator:v0.0.1"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purpose of being used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting addon-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.
	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}

	// Setup ArgoCD before the suite if not skipped and if not already installed
	if !skipArgoCDInstall {
		By("checking if Argo CD is installed already")
		isArgoCDAlreadyInstalled = utils.IsArgoCDInstalled()
		if !isArgoCDAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing Argo CD %s...\n", utils.ArgoCDVersion)
			Expect(utils.InstallArgoCD()).To(Succeed(), "Failed to install Argo CD")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Argo CD is already installed. Skipping installation...\n")
		}
	}

	By("installing CRDs into the cluster")
	Expect(utils.InstallCRDs()).To(Succeed(), "Failed to install CRDs")

	By("deploying the controller-manager")
	Expect(utils.DeployOperator(projectImage)).To(Succeed(), "Failed to deploy the controller-manager")
})

var _ = AfterSuite(func() {
	By("cleaning up test resources before undeploy")
	// Force delete all test resources to prevent undeploy from hanging
	cleanupTestResources()

	By("undeploying the controller-manager")
	utils.UndeployOperator()

	By("uninstalling CRDs from the cluster")
	utils.UninstallCRDs()

	// Teardown ArgoCD after the suite if not skipped and if it was not already installed
	if !skipArgoCDInstall && !isArgoCDAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling Argo CD...\n")
		utils.UninstallArgoCD()
	}

	// Teardown CertManager after the suite if not skipped and if it was not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}
})

// cleanupTestResources forcefully removes all test resources that might have finalizers
// to prevent AfterSuite cleanup from hanging indefinitely.
func cleanupTestResources() {
	// First, patch finalizers on all custom resources to prevent hanging
	patchFinalizers := [][]string{
		{"kubectl", "get", "addons", "-o", "name"},
		{"kubectl", "get", "addonvalues", "-o", "name"},
		{"kubectl", "get", "addonphases", "-o", "name"},
	}

	for _, args := range patchFinalizers {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		output, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			// Patch each resource to remove finalizers
			for _, line := range utils.GetNonEmptyLines(string(output)) {
				patchCtx, patchCancel := context.WithTimeout(context.Background(), 5*time.Second)
				patchCmd := exec.CommandContext(patchCtx, "kubectl", "patch", line,
					"-p", `{"metadata":{"finalizers":[]}}`, "--type=merge")
				_, _ = patchCmd.CombinedOutput()
				patchCancel()
			}
		}
	}

	// Delete all test resources with timeout - ignore errors as resources might not exist
	cmds := [][]string{
		{"kubectl", "delete", "addons", "--all", "--force", "--grace-period=0", "--wait=false"},
		{"kubectl", "delete", "addonvalues", "--all", "--force", "--grace-period=0", "--wait=false"},
		{"kubectl", "delete", "addonphases", "--all", "--force", "--grace-period=0", "--wait=false"},
		{"kubectl", "delete", "applications", "-n", "argocd", "--all", "--force", "--grace-period=0", "--wait=false"},
		{"kubectl", "delete", "deployments", "-n", "default", "--all", "--force", "--grace-period=0", "--wait=false"},
	}

	for _, args := range cmds {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		_, _ = cmd.CombinedOutput()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: cleanup command timed out: %v\n", args)
		}
	}
}
