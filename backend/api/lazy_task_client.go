// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"sync"

	"github.com/cloudwego/kitex/client/callopt"

	"github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/observability/task"
	"github.com/coze-dev/coze-loop/backend/kitex_gen/coze/loop/observability/task/taskservice"
)

// lazyTaskClient defers resolving the underlying task client until first use.
//
// The evaluation handler is wired before the observability handler, but its
// task-client factory captures the (still nil) observability handler. wire's
// provideTaskClient invokes that factory eagerly during wiring, which would
// dereference the nil handler and panic on startup. Wrapping it lazily lets the
// factory return a non-nil client immediately and only touch the observability
// handler once it has been fully initialized (i.e. on the first task RPC).
type lazyTaskClient struct {
	resolve func() taskservice.Client
	once    sync.Once
	client  taskservice.Client
}

func newLazyTaskClient(resolve func() taskservice.Client) *lazyTaskClient {
	return &lazyTaskClient{resolve: resolve}
}

func (l *lazyTaskClient) get() taskservice.Client {
	l.once.Do(func() { l.client = l.resolve() })
	return l.client
}

func (l *lazyTaskClient) CheckTaskName(ctx context.Context, req *task.CheckTaskNameRequest, callOptions ...callopt.Option) (*task.CheckTaskNameResponse, error) {
	return l.get().CheckTaskName(ctx, req, callOptions...)
}

func (l *lazyTaskClient) CreateTask(ctx context.Context, req *task.CreateTaskRequest, callOptions ...callopt.Option) (*task.CreateTaskResponse, error) {
	return l.get().CreateTask(ctx, req, callOptions...)
}

func (l *lazyTaskClient) UpdateTask(ctx context.Context, req *task.UpdateTaskRequest, callOptions ...callopt.Option) (*task.UpdateTaskResponse, error) {
	return l.get().UpdateTask(ctx, req, callOptions...)
}

func (l *lazyTaskClient) ListTasks(ctx context.Context, req *task.ListTasksRequest, callOptions ...callopt.Option) (*task.ListTasksResponse, error) {
	return l.get().ListTasks(ctx, req, callOptions...)
}

func (l *lazyTaskClient) GetTask(ctx context.Context, req *task.GetTaskRequest, callOptions ...callopt.Option) (*task.GetTaskResponse, error) {
	return l.get().GetTask(ctx, req, callOptions...)
}
