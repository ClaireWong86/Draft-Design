// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
export type OptimizeSourceType = 'experiment' | 'eval_set';

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
}

export interface OptimizeCaseDetail {
  case_id: string;
  before_score?: number;
  after_score?: number;
  before_actual?: string;
  after_actual?: string;
  reference?: string;
}

export interface OptimizeTaskResult {
  before_prompt: string;
  after_prompt: string;
  before_score: number;
  after_score: number;
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
  workspace_id: string;
  prompt_id: string;
  prompt_version?: string;
  source_type: OptimizeSourceType;
  source_id: string;
  source_name?: string;
  case_item_ids: string[];
  mapping: OptimizeFieldMapping;
  mode_score: number;
  status: OptimizeTaskStatus;
  progress: number;
  error_msg?: string;
  result?: OptimizeTaskResult;
  created_at: number;
  updated_at: number;
}

export interface CreateOptimizeTaskRequest {
  workspace_id: string;
  prompt_id: string;
  prompt_version?: string;
  source_type: OptimizeSourceType;
  source_id: string;
  source_name?: string;
  case_item_ids: string[];
  mapping: OptimizeFieldMapping;
  mode_score: number;
  baseline_prompt?: string;
}

export interface OptimizeTaskClient {
  createTask: (req: CreateOptimizeTaskRequest) => Promise<OptimizeTask>;
  listTasks: (params: {
    workspace_id: string;
    prompt_id: string;
  }) => Promise<OptimizeTask[]>;
  getTask: (taskId: string) => Promise<OptimizeTask | undefined>;
  cancelTask: (taskId: string) => Promise<void>;
  adoptTask: (taskId: string) => Promise<{ after_prompt: string }>;
}

/** API-shaped list filter (snake_case matches TRD / future IDL). */
export interface ListOptimizeTasksParams {
  workspace_id: string;
  prompt_id: string;
}
