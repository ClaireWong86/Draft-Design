// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import * as prompt_prompt from './../prompt/domain/prompt';
export { prompt_prompt };
import * as base from './../../../base';
export { base };
import { createAPI } from './../../config';
export enum OptimizeSourceType {
  OptimizeSourceTypeExperiment = "experiment",
  OptimizeSourceTypeEvalSet = "eval_set",
}
export enum OptimizeTaskStatus {
  OptimizeTaskStatusQueued = "queued",
  OptimizeTaskStatusRunning = "running",
  OptimizeTaskStatusSucceeded = "succeeded",
  OptimizeTaskStatusNoGain = "no_gain",
  OptimizeTaskStatusFailed = "failed",
  OptimizeTaskStatusCancelled = "cancelled",
}
export interface OptimizeSource {
  type: OptimizeSourceType,
  experiment_id?: string,
  experiment_name?: string,
  eval_set_id?: string,
  eval_set_version_id?: string,
  eval_set_name?: string,
}
export interface OptimizeVariableFieldMapping {
  field_name: string,
  from_field_name: string,
}
export interface OptimizeFieldMapping {
  variable_fields: OptimizeVariableFieldMapping[],
  actual_output_field?: string,
  reference_output_field?: string,
  evaluator_version_id?: string,
  score_min?: number,
  score_max?: number,
  only_failed?: boolean,
}
export interface OptimizePromptSnapshot {
  prompt_id: string,
  prompt_version?: string,
  messages: prompt_prompt.Message[],
  variable_defs?: prompt_prompt.VariableDef[],
}
export interface OptimizeEvaluatorScore {
  evaluator_version_id: string,
  evaluator_name?: string,
  before_score?: number,
  after_score?: number,
}
export interface OptimizeCaseDetail {
  case_id: string,
  before_score?: number,
  after_score?: number,
  before_actual?: string,
  after_actual?: string,
  reference?: string,
  evaluator_scores?: OptimizeEvaluatorScore[],
}
export interface OptimizeDiagnosis {
  failure_modes?: string[],
  suggested_instruction_changes?: string[],
}
export interface OptimizeTaskResult {
  before_prompt: OptimizePromptSnapshot,
  after_prompt: OptimizePromptSnapshot,
  before_score?: number,
  after_score?: number,
  before_score_distribution?: number[],
  after_score_distribution?: number[],
  case_details?: OptimizeCaseDetail[],
  diagnosis?: OptimizeDiagnosis,
}
export interface OptimizeTask {
  id: string,
  name: string,
  workspace_id: string,
  prompt_id: string,
  prompt_version?: string,
  source: OptimizeSource,
  case_item_ids: string[],
  mapping: OptimizeFieldMapping,
  mode_score: number,
  optimizer_model_id: string,
  status: OptimizeTaskStatus,
  progress: number,
  error_msg?: string,
  result?: OptimizeTaskResult,
  created_at?: string,
  updated_at?: string,
  created_by?: string,
}
export interface CreateOptimizeTaskRequest {
  workspace_id: string,
  prompt_id: string,
  name?: string,
  prompt_version?: string,
  source: OptimizeSource,
  case_item_ids: string[],
  mapping: OptimizeFieldMapping,
  mode_score: number,
  optimizer_model_id: string,
  baseline_prompt: OptimizePromptSnapshot,
}
export interface CreateOptimizeTaskResponse {
  task?: OptimizeTask
}
export interface ListOptimizeTasksRequest {
  workspace_id: string,
  prompt_id: string,
  keyword?: string,
  statuses?: OptimizeTaskStatus[],
  page_number?: number,
  page_size?: number,
}
export interface ListOptimizeTasksResponse {
  tasks?: OptimizeTask[],
  total?: string,
}
export interface GetOptimizeTaskRequest {
  workspace_id: string,
  task_id: string,
}
export interface GetOptimizeTaskResponse {
  task?: OptimizeTask
}
export interface CancelOptimizeTaskRequest {
  workspace_id: string,
  task_id: string,
}
export interface CancelOptimizeTaskResponse {}
export const CreateOptimizeTask = /*#__PURE__*/createAPI<CreateOptimizeTaskRequest, CreateOptimizeTaskResponse>({
  "url": "/api/evaluation/v1/prompts/:prompt_id/optimize_tasks",
  "method": "POST",
  "name": "CreateOptimizeTask",
  "reqType": "CreateOptimizeTaskRequest",
  "reqMapping": {
    "body": ["workspace_id", "name", "prompt_version", "source", "case_item_ids", "mapping", "mode_score", "optimizer_model_id", "baseline_prompt"],
    "path": ["prompt_id"]
  },
  "resType": "CreateOptimizeTaskResponse",
  "schemaRoot": "api://schemas/evaluation_coze.loop.evaluation.optimize",
  "service": "evaluationOptimize"
});
export const ListOptimizeTasks = /*#__PURE__*/createAPI<ListOptimizeTasksRequest, ListOptimizeTasksResponse>({
  "url": "/api/evaluation/v1/prompts/:prompt_id/optimize_tasks/list",
  "method": "POST",
  "name": "ListOptimizeTasks",
  "reqType": "ListOptimizeTasksRequest",
  "reqMapping": {
    "body": ["workspace_id", "keyword", "statuses", "page_number", "page_size"],
    "path": ["prompt_id"]
  },
  "resType": "ListOptimizeTasksResponse",
  "schemaRoot": "api://schemas/evaluation_coze.loop.evaluation.optimize",
  "service": "evaluationOptimize"
});
export const GetOptimizeTask = /*#__PURE__*/createAPI<GetOptimizeTaskRequest, GetOptimizeTaskResponse>({
  "url": "/api/evaluation/v1/optimize_tasks/:task_id",
  "method": "GET",
  "name": "GetOptimizeTask",
  "reqType": "GetOptimizeTaskRequest",
  "reqMapping": {
    "query": ["workspace_id"],
    "path": ["task_id"]
  },
  "resType": "GetOptimizeTaskResponse",
  "schemaRoot": "api://schemas/evaluation_coze.loop.evaluation.optimize",
  "service": "evaluationOptimize"
});
export const CancelOptimizeTask = /*#__PURE__*/createAPI<CancelOptimizeTaskRequest, CancelOptimizeTaskResponse>({
  "url": "/api/evaluation/v1/optimize_tasks/:task_id/cancel",
  "method": "POST",
  "name": "CancelOptimizeTask",
  "reqType": "CancelOptimizeTaskRequest",
  "reqMapping": {
    "body": ["workspace_id"],
    "path": ["task_id"]
  },
  "resType": "CancelOptimizeTaskResponse",
  "schemaRoot": "api://schemas/evaluation_coze.loop.evaluation.optimize",
  "service": "evaluationOptimize"
});