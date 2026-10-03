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

package addonclaim

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonsv1alpha1 "addons-operator/api/v1alpha1"
	"addons-operator/internal/controller/addonclaim/remoteclient"
	pkgconditions "addons-operator/pkg/conditions"
)

var addonGR = schema.GroupResource{Group: "addons.in-cloud.io", Resource: "addons"}

// Ошибки взяты в той форме, в какой их отдаёт client-go: обёрнутыми, как в reconcileRemoteAddon.
func TestClassifyRemoteError(t *testing.T) {
	noKind := &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: "addons.in-cloud.io", Kind: "Addon"},
		SearchedVersions: []string{"v1alpha1"},
	}
	webhook := apierrors.NewInternalError(errors.New(
		`failed calling webhook "vaddon-v1alpha1.kb.io": failed to call webhook: Post "https://svc:443/validate": dial tcp: connect: connection refused`))
	dialRefused := &url.Error{Op: "Get", URL: "https://10.0.0.1:6443/apis", Err: &net.OpError{
		Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}

	cases := []struct {
		name string
		err  error
		want remoteErrClass
	}{
		{"CRD ещё нет", fmt.Errorf("get remote Addon: %w", noKind), remoteNotReady},
		{"отказ вызова вебхука", webhook, remoteNotReady},
		{"конфликт версии", apierrors.NewConflict(addonGR, "x", errors.New("modified")), remoteNotReady},
		{"уже создан", apierrors.NewAlreadyExists(addonGR, "x"), remoteNotReady},
		{"нет соединения", fmt.Errorf("get remote Addon: %w", dialRefused), remoteUnreachable},
		{"таймаут клиента", context.DeadlineExceeded, remoteUnreachable},
		{"ServerTimeout (500, другая причина)", apierrors.NewServerTimeout(addonGR, "create", 5), remoteUnreachable},
		{"504", apierrors.NewTimeoutError("timeout", 5), remoteUnreachable},
		{"503", apierrors.NewServiceUnavailable("down"), remoteUnreachable},
		{"502 от балансировщика", apierrors.NewGenericServerResponse(502, "get", addonGR, "x", "bad gateway", 0, false), remoteUnreachable},
		{"429", apierrors.NewTooManyRequests("slow down", 1), remoteUnreachable},
		{"401", apierrors.NewUnauthorized("token"), remoteUnreachable},
		{"403", apierrors.NewForbidden(addonGR, "x", errors.New("rbac")), remoteUnreachable},
		{"422", apierrors.NewInvalid(schema.GroupKind{Group: "addons.in-cloud.io", Kind: "Addon"}, "x", nil), remoteRejected},
		{"400", apierrors.NewBadRequest("bad"), remoteRejected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyRemoteError(tc.err); got != tc.want {
				t.Errorf("classifyRemoteError(%v) = %d, ждали %d", tc.err, got, tc.want)
			}
		})
	}
}

func TestNotReadyRequeue(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cond := func(status metav1.ConditionStatus, ago time.Duration) *metav1.Condition {
		return &metav1.Condition{Type: TypeAddonSynced, Status: status, LastTransitionTime: metav1.NewTime(now.Add(-ago))}
	}
	cases := []struct {
		name string
		c    *metav1.Condition
		want time.Duration
	}{
		{"условия нет", nil, notReadyRetryInterval},
		{"только что перешло в False", cond(metav1.ConditionFalse, time.Second), notReadyRetryInterval},
		{"False меньше окна", cond(metav1.ConditionFalse, notReadyFastWindow-time.Second), notReadyRetryInterval},
		{"False дольше окна", cond(metav1.ConditionFalse, notReadyFastWindow+time.Second), notReadyRetryIntervalSlow},
		{"было True — отсчёт ещё не начат", cond(metav1.ConditionTrue, time.Hour), notReadyRetryInterval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := notReadyRequeue(tc.c, now); got != tc.want {
				t.Errorf("notReadyRequeue = %s, ждали %s", got, tc.want)
			}
		})
	}
}

// Отказ «кластер не готов» не открывает общую паузу и даёт короткий повтор; недоступность — открывает.
func TestHandleRemoteSyncError_Backoff(t *testing.T) {
	noKind := fmt.Errorf("get remote Addon: %w", &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{Group: "addons.in-cloud.io", Kind: "Addon"}})
	dialRefused := &url.Error{Op: "Get", URL: "https://10.0.0.1:6443", Err: syscall.ECONNREFUSED}

	cases := []struct {
		name         string
		err          error
		wantPause    bool
		wantRequeue  time.Duration
		wantReason   string
		wantDegraded bool
	}{
		{"CRD ещё нет", noKind, false, notReadyRetryInterval, ReasonRemoteNotReady, false},
		{"нет соединения (контроль)", dialRefused, true, requeueIntervalDegraded, ReasonRemoteOperationFail, true},
		{"запись отвергнута", apierrors.NewBadRequest("bad"), false, requeueIntervalDegraded, ReasonRemoteRequestInvalid, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := addonsv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			claim := &addonsv1alpha1.AddonClaim{ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns", Generation: 1}}
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(claim).
				WithStatusSubresource(&addonsv1alpha1.AddonClaim{}).Build()
			r := &Reconciler{Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
				RemoteClients: remoteclient.NewCache(scheme)}
			key := remoteclient.CacheKey{Namespace: "ns", SecretName: "kc"}
			cm := pkgconditions.New(&claim.Status.Conditions, claim.Generation)
			cm.EnsureAllConditions()
			rctx := &reconcileContext{claim: claim, cm: cm, cacheKey: key, oldStatus: *claim.Status.DeepCopy()}

			res, err := r.handleRemoteSyncError(context.Background(), rctx, tc.err, "Addon")
			if err != nil {
				t.Fatalf("handleRemoteSyncError: %v", err)
			}
			if ok, _ := r.RemoteClients.CheckHealth(key); ok == tc.wantPause {
				t.Errorf("пауза кластера открыта=%v, ждали %v", !ok, tc.wantPause)
			}
			if res.RequeueAfter != tc.wantRequeue {
				t.Errorf("RequeueAfter = %s, ждали %s", res.RequeueAfter, tc.wantRequeue)
			}
			if c := cm.GetCondition(TypeAddonSynced); c == nil || c.Reason != tc.wantReason {
				t.Errorf("AddonSynced.Reason = %v, ждали %s", c, tc.wantReason)
			}
			if cm.IsDegraded() != tc.wantDegraded {
				t.Errorf("Degraded = %v, ждали %v", cm.IsDegraded(), tc.wantDegraded)
			}
		})
	}
}

// Частый опрос — только у заявки control-plane (external-status) после создания удалённого
// Application и до развёртывания; остальные — прежний интервал.
func TestReadinessPollInterval(t *testing.T) {
	r := &Reconciler{}
	appCreated := &addonsv1alpha1.RemoteAddonStatus{Conditions: []metav1.Condition{
		{Type: "ApplicationCreated", Status: metav1.ConditionTrue}}}
	appNotYet := &addonsv1alpha1.RemoteAddonStatus{Conditions: []metav1.Condition{
		{Type: "ApplicationCreated", Status: metav1.ConditionFalse}}}
	ext := map[string]string{"external-status/type": "ControlPlane"}

	cases := []struct {
		name     string
		ann      map[string]string
		deployed bool
		remote   *addonsv1alpha1.RemoteAddonStatus
		want     time.Duration
	}{
		{"control-plane, Application создан", ext, false, appCreated, externalStatusPollInterval},
		{"control-plane, Application ещё нет", ext, false, appNotYet, DefaultPollingInterval},
		{"control-plane, статуса нет", ext, false, nil, DefaultPollingInterval},
		{"control-plane, уже развёрнут", ext, true, appCreated, DefaultPollingInterval},
		{"обычная заявка (контроль)", nil, false, appCreated, DefaultPollingInterval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claim := &addonsv1alpha1.AddonClaim{ObjectMeta: metav1.ObjectMeta{Annotations: tc.ann}}
			claim.Status.Deployed = tc.deployed
			claim.Status.RemoteAddonStatus = tc.remote
			if got := r.readinessPollInterval(claim); got != tc.want {
				t.Errorf("readinessPollInterval = %s, ждали %s", got, tc.want)
			}
		})
	}
}
