// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import "sync"

// OptimizeTaskWaker enqueues a durable OptimizeTask for local processing.
// Callers must never bypass MarkRunning lease claims.
type OptimizeTaskWaker interface {
	WakeTask(taskID int64)
}

var (
	optimizeTaskWakerMu sync.RWMutex
	optimizeTaskWaker   OptimizeTaskWaker
)

// RegisterOptimizeTaskWaker registers the in-process optimize worker wake target.
func RegisterOptimizeTaskWaker(waker OptimizeTaskWaker) {
	optimizeTaskWakerMu.Lock()
	defer optimizeTaskWakerMu.Unlock()
	optimizeTaskWaker = waker
}

// WakeOptimizeTask enqueues taskID on the registered local worker, if any.
func WakeOptimizeTask(taskID int64) {
	if taskID <= 0 {
		return
	}
	optimizeTaskWakerMu.RLock()
	waker := optimizeTaskWaker
	optimizeTaskWakerMu.RUnlock()
	if waker == nil {
		return
	}
	waker.WakeTask(taskID)
}

// WakeTask implements OptimizeTaskWaker. It only enqueues; lease CAS decides ownership.
func (a *OptimizeApplication) WakeTask(taskID int64) {
	if a == nil || taskID <= 0 {
		return
	}
	a.enqueue(taskID)
}
