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

package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

const (
	argoCDCRDName   = "applications.argoproj.io"
	crdCheckTimeout = 5 * time.Second
)

// CheckArgoCDCRD returns a healthz.Checker that verifies the ArgoCD Application CRD exists.
// This ensures the operator doesn't report ready when ArgoCD is not installed.
func CheckArgoCDCRD(reader client.Reader) healthz.Checker {
	return func(req *http.Request) error {
		ctx, cancel := context.WithTimeout(req.Context(), crdCheckTimeout)
		defer cancel()

		crd := &apiextensionsv1.CustomResourceDefinition{}
		err := reader.Get(ctx, types.NamespacedName{Name: argoCDCRDName}, crd)

		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("ArgoCD CRD not installed: %s not found", argoCDCRDName)
			}
			return fmt.Errorf("check ArgoCD CRD: %w", err)
		}

		return nil
	}
}
