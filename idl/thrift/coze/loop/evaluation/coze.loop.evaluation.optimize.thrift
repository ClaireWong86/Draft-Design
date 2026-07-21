namespace go coze.loop.evaluation.optimize

include "../../../base.thrift"
include "../prompt/domain/prompt.thrift"

typedef string OptimizeSourceType (ts.enum="true")
const OptimizeSourceType OptimizeSourceTypeExperiment = "experiment"
const OptimizeSourceType OptimizeSourceTypeEvalSet = "eval_set"

typedef string OptimizeTaskStatus (ts.enum="true")
const OptimizeTaskStatus OptimizeTaskStatusQueued = "queued"
const OptimizeTaskStatus OptimizeTaskStatusRunning = "running"
const OptimizeTaskStatus OptimizeTaskStatusSucceeded = "succeeded"
const OptimizeTaskStatus OptimizeTaskStatusFailed = "failed"
const OptimizeTaskStatus OptimizeTaskStatusCancelled = "cancelled"

struct OptimizeSource {
    1: required OptimizeSourceType type
    2: optional i64 experiment_id (api.js_conv="true", go.tag='json:"experiment_id"')
    3: optional string experiment_name
    4: optional i64 eval_set_id (api.js_conv="true", go.tag='json:"eval_set_id"')
    5: optional i64 eval_set_version_id (api.js_conv="true", go.tag='json:"eval_set_version_id"')
    6: optional string eval_set_name
}

struct OptimizeVariableFieldMapping {
    1: required string field_name
    2: required string from_field_name
}

struct OptimizeFieldMapping {
    1: required list<OptimizeVariableFieldMapping> variable_fields
    2: optional string actual_output_field
    3: optional string reference_output_field
    4: optional i64 evaluator_version_id (api.js_conv="true", go.tag='json:"evaluator_version_id"')
    5: optional double score_min
    6: optional double score_max
    7: optional bool only_failed
}

struct OptimizePromptSnapshot {
    1: required i64 prompt_id (api.js_conv="true", go.tag='json:"prompt_id"')
    2: optional string prompt_version
    3: required list<prompt.Message> messages
    4: optional list<prompt.VariableDef> variable_defs
}

struct OptimizeEvaluatorScore {
    1: required i64 evaluator_version_id (api.js_conv="true", go.tag='json:"evaluator_version_id"')
    2: optional string evaluator_name
    3: optional double before_score
    4: optional double after_score
}

struct OptimizeCaseDetail {
    1: required string case_id
    2: optional double before_score
    3: optional double after_score
    4: optional string before_actual
    5: optional string after_actual
    6: optional string reference
    7: optional list<OptimizeEvaluatorScore> evaluator_scores
}

struct OptimizeDiagnosis {
    1: optional list<string> failure_modes
    2: optional list<string> suggested_instruction_changes
}

struct OptimizeTaskResult {
    1: required OptimizePromptSnapshot before_prompt
    2: required OptimizePromptSnapshot after_prompt
    3: optional double before_score
    4: optional double after_score
    5: optional list<double> before_score_distribution
    6: optional list<double> after_score_distribution
    7: optional list<OptimizeCaseDetail> case_details
    8: optional OptimizeDiagnosis diagnosis
}

struct OptimizeTask {
    1: required i64 id (api.js_conv="true", go.tag='json:"id"')
    2: required string name
    3: required i64 workspace_id (api.js_conv="true", go.tag='json:"workspace_id"')
    4: required i64 prompt_id (api.js_conv="true", go.tag='json:"prompt_id"')
    5: optional string prompt_version
    6: required OptimizeSource source
    7: required list<string> case_item_ids
    8: required OptimizeFieldMapping mapping
    9: required double mode_score
    10: required i64 optimizer_model_id (api.js_conv="true", go.tag='json:"optimizer_model_id"')
    11: required OptimizeTaskStatus status
    12: required i32 progress
    13: optional string error_msg
    14: optional OptimizeTaskResult result
    15: optional i64 created_at (api.js_conv="true", go.tag='json:"created_at"')
    16: optional i64 updated_at (api.js_conv="true", go.tag='json:"updated_at"')
    17: optional string created_by
}

struct CreateOptimizeTaskRequest {
    1: required i64 workspace_id (api.js_conv="true", go.tag='json:"workspace_id"')
    2: required i64 prompt_id (api.path="prompt_id", api.js_conv="true", go.tag='json:"prompt_id"')
    3: optional string name
    4: optional string prompt_version
    5: required OptimizeSource source
    6: required list<string> case_item_ids (vt.min_size="1", vt.max_size="500")
    7: required OptimizeFieldMapping mapping
    8: required double mode_score
    9: required i64 optimizer_model_id (api.js_conv="true", go.tag='json:"optimizer_model_id"', vt.gt="0")
    10: required OptimizePromptSnapshot baseline_prompt
    255: optional base.Base base
}

struct CreateOptimizeTaskResponse {
    1: optional OptimizeTask task
    255: optional base.BaseResp BaseResp
}

struct ListOptimizeTasksRequest {
    1: required i64 workspace_id (api.js_conv="true", go.tag='json:"workspace_id"')
    2: required i64 prompt_id (api.path="prompt_id", api.js_conv="true", go.tag='json:"prompt_id"')
    3: optional string keyword
    4: optional list<OptimizeTaskStatus> statuses
    101: optional i32 page_number
    102: optional i32 page_size
    255: optional base.Base base
}

struct ListOptimizeTasksResponse {
    1: optional list<OptimizeTask> tasks
    2: optional i64 total (api.js_conv="true", go.tag='json:"total"')
    255: optional base.BaseResp BaseResp
}

struct GetOptimizeTaskRequest {
    1: required i64 workspace_id (api.query="workspace_id", api.js_conv="true", go.tag='json:"workspace_id"')
    2: required i64 task_id (api.path="task_id", api.js_conv="true", go.tag='json:"task_id"')
    255: optional base.Base base
}

struct GetOptimizeTaskResponse {
    1: optional OptimizeTask task
    255: optional base.BaseResp BaseResp
}

struct CancelOptimizeTaskRequest {
    1: required i64 workspace_id (api.js_conv="true", go.tag='json:"workspace_id"')
    2: required i64 task_id (api.path="task_id", api.js_conv="true", go.tag='json:"task_id"')
    255: optional base.Base base
}

struct CancelOptimizeTaskResponse {
    255: optional base.BaseResp BaseResp
}

service OptimizeService {
    CreateOptimizeTaskResponse CreateOptimizeTask(1: CreateOptimizeTaskRequest req) (api.post="/api/evaluation/v1/prompts/:prompt_id/optimize_tasks")
    ListOptimizeTasksResponse ListOptimizeTasks(1: ListOptimizeTasksRequest req) (api.post="/api/evaluation/v1/prompts/:prompt_id/optimize_tasks/list")
    GetOptimizeTaskResponse GetOptimizeTask(1: GetOptimizeTaskRequest req) (api.get="/api/evaluation/v1/optimize_tasks/:task_id")
    CancelOptimizeTaskResponse CancelOptimizeTask(1: CancelOptimizeTaskRequest req) (api.post="/api/evaluation/v1/optimize_tasks/:task_id/cancel")
}
