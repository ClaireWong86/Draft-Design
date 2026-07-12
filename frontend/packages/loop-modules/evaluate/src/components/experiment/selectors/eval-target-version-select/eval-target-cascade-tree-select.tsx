// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { I18n } from '@cozeloop/i18n-adapter';
import { useSpace } from '@cozeloop/biz-hooks-adapter';
import { EvalTargetType } from '@cozeloop/api-schema/evaluation';
import { Select } from '@coze-arch/coze-design';

import PromptEvalTargetTreeSelect from './prompt-eval-target-tree-select';

interface EvalTargetSelectValue {
  evalTargetType: EvalTargetType;
  ids: string[];
}

export default function EvalTargetCascadeTreeSelect({
  value,
  onChange,
}: {
  value?: EvalTargetSelectValue | undefined;
  onChange?: (val: EvalTargetSelectValue) => void;
}) {
  const { spaceID } = useSpace();
  const evalTargetType = value?.evalTargetType ?? EvalTargetType.CozeLoopPrompt;
  const evalTargetSelect = (
    <PromptEvalTargetTreeSelect
      spaceID={spaceID}
      value={value?.ids}
      onChange={newKeys => {
        onChange?.({
          evalTargetType:
            value?.evalTargetType ?? EvalTargetType.CozeLoopPrompt,
          ids: newKeys,
        });
      }}
    />
  );
  return (
    <div className="flex items-center gap-1">
      <Select
        className="!w-24 shrink-0"
        placeholder={I18n.t('evaluate_target_type')}
        value={evalTargetType}
        showArrow={false}
        onChange={val => {
          onChange?.({
            evalTargetType: val as EvalTargetType,
            ids: [],
          });
        }}
        optionList={[{ label: 'Prompt', value: EvalTargetType.CozeLoopPrompt }]}
      />

      <div className="grow">{evalTargetSelect}</div>
    </div>
  );
}
