// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { StoneEvaluationApi } from '@cozeloop/api-schema';
import type { evaluationOptimize } from '@cozeloop/api-schema';

import {
  type CreateOptimizeTaskRequest,
  type OptimizeSource,
  type OptimizeTask,
  type OptimizeTaskClient,
  type OptimizeTaskResult,
  type OptimizeTaskStatus,
} from './types';

function toSource(source: evaluationOptimize.OptimizeSource): OptimizeSource {
  if (String(source.type) === 'experiment') {
    return {
      type: 'experiment',
      experiment_id: source.experiment_id || '',
      experiment_name: source.experiment_name,
    };
  }
  return {
    type: 'eval_set',
    eval_set_id: source.eval_set_id || '',
    eval_set_version_id: source.eval_set_version_id || '',
    eval_set_name: source.eval_set_name,
  };
}

function toResult(
  result?: evaluationOptimize.OptimizeTaskResult,
): OptimizeTaskResult | undefined {
  if (!result) {
    return undefined;
  }
  return {
    before_prompt: result.before_prompt,
    after_prompt: result.after_prompt,
    before_score: result.before_score,
    after_score: result.after_score,
    score_distribution: {
      before: result.before_score_distribution || [],
      after: result.after_score_distribution || [],
    },
    case_details: result.case_details || [],
    diagnosis: result.diagnosis
      ? {
          failure_modes: result.diagnosis.failure_modes || [],
          suggested_instruction_changes:
            result.diagnosis.suggested_instruction_changes || [],
        }
      : undefined,
  };
}

function toTask(task: evaluationOptimize.OptimizeTask): OptimizeTask {
  return {
    id: task.id,
    name: task.name,
    workspace_id: task.workspace_id,
    prompt_id: task.prompt_id,
    prompt_version: task.prompt_version,
    source: toSource(task.source),
    case_item_ids: task.case_item_ids,
    mapping: task.mapping,
    mode_score: task.mode_score,
    optimizer_model_id: task.optimizer_model_id,
    status: String(task.status) as OptimizeTaskStatus,
    progress: task.progress,
    error_msg: task.error_msg,
    result: toResult(task.result),
    created_at: Number(task.created_at || 0),
    updated_at: Number(task.updated_at || 0),
  };
}

function toCreateRequest(
  req: CreateOptimizeTaskRequest,
): evaluationOptimize.CreateOptimizeTaskRequest {
  return {
    ...req,
    source: {
      ...req.source,
      type: req.source.type as evaluationOptimize.OptimizeSourceType,
    },
  };
}

export const optimizeTaskClient: OptimizeTaskClient = {
  async createTask(req) {
    const { task } = await StoneEvaluationApi.CreateOptimizeTask(
      toCreateRequest(req),
    );
    if (!task) {
      throw new Error('创建智能优化任务失败');
    }
    return toTask(task);
  },

  async listTasks(params) {
    const { tasks } = await StoneEvaluationApi.ListOptimizeTasks({
      ...params,
      page_number: 1,
      page_size: 100,
    });
    return (tasks || []).map(toTask);
  },

  async getTask(taskId, workspaceId = '') {
    if (!workspaceId) {
      return undefined;
    }
    const { task } = await StoneEvaluationApi.GetOptimizeTask({
      task_id: taskId,
      workspace_id: workspaceId,
    });
    return task ? toTask(task) : undefined;
  },

  async cancelTask(taskId, workspaceId) {
    await StoneEvaluationApi.CancelOptimizeTask({
      task_id: taskId,
      workspace_id: workspaceId,
    });
  },

  async adoptTask(taskId, workspaceId) {
    const task = await this.getTask(taskId, workspaceId);
    if (!task?.result?.after_prompt) {
      throw new Error('任务尚无可用优化结果');
    }
    return { after_prompt: task.result.after_prompt };
  },
};
