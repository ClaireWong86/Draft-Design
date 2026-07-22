// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"sync"
	"time"

	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
)

// OptimizeTaskWakePublisher publishes optional cross-instance wake signals.
// Implementations must treat a disabled / missing RMQ producer as a no-op success.
type OptimizeTaskWakePublisher interface {
	PublishOptimizeTaskWakeEvent(ctx context.Context, event *entity.OptimizeTaskWakeEvent, duration *time.Duration) error
}

var (
	optimizeWakePublisherMu sync.RWMutex
	optimizeWakePublisher   OptimizeTaskWakePublisher
)

// SetOptimizeTaskWakePublisher registers the process-wide wake publisher.
// Infra MQ producers call this during initialization; tests may inject a stub.
func SetOptimizeTaskWakePublisher(publisher OptimizeTaskWakePublisher) {
	optimizeWakePublisherMu.Lock()
	defer optimizeWakePublisherMu.Unlock()
	optimizeWakePublisher = publisher
}

// PublishOptimizeTaskWakeBestEffort notifies other instances that a queued task
// is ready. Failures are ignored: local enqueue and the MySQL resume scan recover.
func PublishOptimizeTaskWakeBestEffort(ctx context.Context, taskID int64) {
	if taskID <= 0 {
		return
	}
	optimizeWakePublisherMu.RLock()
	publisher := optimizeWakePublisher
	optimizeWakePublisherMu.RUnlock()
	if publisher == nil {
		return
	}
	_ = publisher.PublishOptimizeTaskWakeEvent(ctx, &entity.OptimizeTaskWakeEvent{TaskID: taskID}, nil)
}
