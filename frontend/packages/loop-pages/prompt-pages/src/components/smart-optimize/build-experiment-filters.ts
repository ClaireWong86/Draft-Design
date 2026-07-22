// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import {
  EvalTargetType,
  ExptStatus,
  FieldType,
  FilterLogicOp,
  FilterOperatorType,
  type Filters,
} from '@cozeloop/api-schema/evaluation';

/** ListExperiments filters: success + Prompt target for current prompt. */
export function buildSmartOptimizeExperimentFilters(
  promptID?: string,
): Filters | undefined {
  if (!promptID) {
    return undefined;
  }
  return {
    logic_op: FilterLogicOp.And,
    filter_conditions: [
      {
        field: { field_type: FieldType.ExptStatus },
        operator: FilterOperatorType.In,
        value: String(ExptStatus.Success),
      },
      {
        field: { field_type: FieldType.SourceTarget },
        operator: FilterOperatorType.In,
        value: '',
        source_target: {
          eval_target_type: EvalTargetType.CozeLoopPrompt,
          source_target_ids: [promptID],
        },
      },
    ],
  };
}
