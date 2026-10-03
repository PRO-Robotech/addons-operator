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
	"errors"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// remoteErrClass — что отказ записи в удалённый кластер говорит о самом кластере (K8S-1087).
type remoteErrClass int

const (
	// remoteUnreachable — кластер не ответил либо отказал как целое: нет соединения, таймаут,
	// 502/503/504, 429, отказ учётных данных. Только такой отказ двигает общую паузу кластера.
	remoteUnreachable remoteErrClass = iota
	// remoteNotReady — apiserver ответил, но кластер ещё не готов принять запись: CRD не
	// установлен, вебхук не отвечает, конфликт версии. Штатно при сборке; повторяется
	// только эта заявка, коротким интервалом.
	remoteNotReady
	// remoteRejected — apiserver отверг саму запись (400, 422): дело в заявке, а не в кластере.
	remoteRejected
)

const (
	// Короткий повтор заявки, пока кластер не готов: столько обычно длится установка CRD
	// и перекат вебхука оператора.
	notReadyRetryInterval = 5 * time.Second
	// Дольше этого «не готов» уже не похоже на сборку — повтор реже, чтобы не нагружать кластер.
	notReadyFastWindow         = 3 * time.Minute
	notReadyRetryIntervalSlow  = 30 * time.Second
	ReasonRemoteNotReady       = "RemoteNotReady"
	ReasonRemoteRequestInvalid = "RemoteRequestInvalid"
)

// classifyRemoteError относит отказ записи AddonValue или Addon к одному из классов.
// Ответ, не являющийся статусом apiserver (сетевые ошибки, таймаут клиента), — недоступность.
func classifyRemoteError(err error) remoteErrClass {
	// Тип не найден в обнаружении API: группа отвечает, CRD ещё нет.
	if meta.IsNoMatchError(err) {
		return remoteNotReady
	}

	var status apierrors.APIStatus
	if !errors.As(err, &status) {
		return remoteUnreachable
	}

	switch {
	case apierrors.IsConflict(err), apierrors.IsAlreadyExists(err), apierrors.IsNotFound(err):
		return remoteNotReady
	// InternalError с кодом 500 — ответ самого apiserver; так приходит отказ вызова вебхука,
	// пока его под перекатывается. ServerTimeout имеет тот же код, но другую причину, а
	// 502 балансировщика client-go тоже называет InternalError — его отсекает код.
	case apierrors.IsInternalError(err) && status.Status().Code == http.StatusInternalServerError:
		return remoteNotReady
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err):
		return remoteRejected
	default:
		return remoteUnreachable
	}
}

// notReadyRequeue — интервал повтора заявки, пока кластер не готов. Отсчёт идёт от перехода
// AddonSynced в False, поэтому переживает перезапуск контроллера без счётчика в памяти.
func notReadyRequeue(synced *metav1.Condition, now time.Time) time.Duration {
	if synced == nil || synced.Status != metav1.ConditionFalse {
		return notReadyRetryInterval
	}
	if now.Sub(synced.LastTransitionTime.Time) < notReadyFastWindow {
		return notReadyRetryInterval
	}

	return notReadyRetryIntervalSlow
}
