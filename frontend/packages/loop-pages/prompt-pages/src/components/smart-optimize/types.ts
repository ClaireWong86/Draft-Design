// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { type Message, type VariableDef } from '@cozeloop/api-schema/prompt';

export type OptimizeSourceType = 'experiment' | 'eval_set';

export type OptimizeSource =
  | {
      type: 'experiment';
      experiment_id: string;
      experiment_name?: string;
    }
  | {
      type: 'eval_set';
      eval_set_id: string;
      eval_set_version_id: string;
      eval_set_name?: string;
    };

/**
 * Immutable prompt snapshot used by an optimization task.
 * Keep the original message/part structure so few-shot and VLM inputs survive.
 */
export interface OptimizePromptSnapshot {
  prompt_id: string;
  prompt_version?: string;
  messages: Message[];
  variable_defs?: VariableDef[];
}

export type OptimizeTaskStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'cancelled';

export interface OptimizeFieldMapping {
  variable_fields: Array<{ field_name: string; from_field_name: string }>;
  actual_output_field?: string;
  reference_output_field?: string;
  evaluator_version_id?: string;
  score_min?: number;
  score_max?: number;
  only_failed?: boolean;
}

export interface OptimizeCaseDetail {
  case_id: string;
  before_score?: number;
  after_score?: number;
  before_actual?: string;
  after_actual?: string;
  reference?: string;
  evaluator_scores?: Array<{
    evaluator_version_id: string;
    evaluator_name?: string;
    before_score?: number;
    after_score?: number;
  }>;
}

export interface OptimizeTaskResult {
  before_prompt: OptimizePromptSnapshot;
  after_prompt: OptimizePromptSnapshot;
  before_score?: number;
  after_score?: number;
  score_distribution: {
    before: number[];
    after: number[];
  };
  case_details: OptimizeCaseDetail[];
  diagnosis?: {
    failure_modes: string[];
    suggested_instruction_changes: string[];
  };
}

export interface OptimizeTask {
  id: string;
  name: string;
  workspace_id: string;
  prompt_id: string;
  prompt_version?: string;
  source: OptimizeSource;
  case_item_ids: string[];
  mapping: OptimizeFieldMapping;
  mode_score: number;
  optimizer_model_id: string;
  status: OptimizeTaskStatus;
  progress: number;
  error_msg?: string;
  result?: OptimizeTaskResult;
  created_at: number;
  updated_at: number;
}

export interface CreateOptimizeTaskRequest {
  name?: string;
  workspace_id: string;
  prompt_id: string;
  prompt_version?: string;
  source: OptimizeSource;
  case_item_ids: string[];
  mapping: OptimizeFieldMapping;
  mode_score: number;
  optimizer_model_id: string;
  baseline_prompt: OptimizePromptSnapshot;
}

export interface OptimizeTaskClient {
  createTask: (req: CreateOptimizeTaskRequest) => Promise<OptimizeTask>;
  listTasks: (params: {
    workspace_id: string;
    prompt_id: string;
  }) => Promise<OptimizeTask[]>;
  getTask: (
    taskId: string,
    workspaceId?: string,
  ) => Promise<OptimizeTask | undefined>;
  cancelTask: (taskId: string, workspaceId: string) => Promise<void>;
  adoptTask: (
    taskId: string,
    workspaceId: string,
  ) => Promise<{ after_prompt: OptimizePromptSnapshot }>;
}

/** API-shaped list filter (snake_case matches TRD / future IDL). */
export interface ListOptimizeTasksParams {
  workspace_id: string;
  prompt_id: string;
}
