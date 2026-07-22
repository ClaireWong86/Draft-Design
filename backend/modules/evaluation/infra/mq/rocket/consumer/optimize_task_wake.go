// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"

	"github.com/coze-dev/coze-loop/backend/infra/mq"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/application"
	"github.com/coze-dev/coze-loop/backend/modules/evaluation/domain/entity"
	"github.com/coze-dev/coze-loop/backend/pkg/json"
	"github.com/coze-dev/coze-loop/backend/pkg/lang/conv"
	"github.com/coze-dev/coze-loop/backend/pkg/logs"
)

func NewOptimizeTaskWakeConsumer() mq.IConsumerHandler {
	return &OptimizeTaskWakeConsumer{}
}

type OptimizeTaskWakeConsumer struct{}

func (c *OptimizeTaskWakeConsumer) HandleMessage(ctx context.Context, msg *mq.MessageExt) error {
	event := &entity.OptimizeTaskWakeEvent{}
	if err := json.Unmarshal(msg.Body, event); err != nil {
		logs.CtxError(ctx, "OptimizeTaskWakeEvent json unmarshal fail, raw: %v, err: %s", conv.UnsafeBytesToString(msg.Body), err)
		return nil
	}
	if event.TaskID <= 0 {
		logs.CtxWarn(ctx, "OptimizeTaskWakeEvent missing task_id, msg_id: %v", msg.MsgID)
		return nil
	}
	logs.CtxInfo(ctx, "OptimizeTaskWakeConsumer enqueue task_id=%d msg_id=%v", event.TaskID, msg.MsgID)
	// Only wake the local in-process queue. MarkRunning lease remains source of truth.
	application.WakeOptimizeTask(event.TaskID)
	return nil
}
