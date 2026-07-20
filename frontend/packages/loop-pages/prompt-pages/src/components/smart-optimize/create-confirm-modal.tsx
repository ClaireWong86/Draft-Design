// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';

import { I18n } from '@cozeloop/i18n-adapter';
import { ExperimentsSelect } from '@cozeloop/evaluate-components';
import { type Prompt } from '@cozeloop/api-schema/prompt';
import { Modal, Typography } from '@coze-arch/coze-design';

interface SmartOptimizeCreateConfirmModalProps {
  visible: boolean;
  prompt?: Prompt;
  onCancel: () => void;
  onConfirm: (experimentID: string) => void;
}

export function SmartOptimizeCreateConfirmModal({
  visible,
  prompt,
  onCancel,
  onConfirm,
}: SmartOptimizeCreateConfirmModalProps) {
  const [experimentID, setExperimentID] = useState('');
  const version =
    prompt?.prompt_commit?.commit_info?.version ||
    prompt?.prompt_basic?.latest_version;

  const handleCancel = () => {
    setExperimentID('');
    onCancel();
  };

  return (
    <Modal
      title={I18n.t('smart_optimize_create', '新建智能优化')}
      visible={visible}
      width={720}
      okText={I18n.t('confirm', '确定')}
      cancelText={I18n.t('cancel', '取消')}
      okButtonProps={{ disabled: !experimentID }}
      onCancel={handleCancel}
      onOk={() => onConfirm(experimentID)}
    >
      <div className="mb-6 rounded-lg bg-[var(--coz-mg-hglt)] p-4">
        <Typography.Text>
          {I18n.t(
            'smart_optimize_experiment_intro',
            '基于批量评测的问题、模型回答、参考答案（如有）以及对应评估器得分来优化 Prompt',
          )}
        </Typography.Text>
      </div>
      <div className="flex flex-col gap-6">
        <div>
          <Typography.Text strong>
            {I18n.t('smart_optimize_selected_experiment', '选择评测实验')}
            <span className="ml-1 text-red-500">*</span>
          </Typography.Text>
          <ExperimentsSelect
            className="mt-2 w-full"
            value={experimentID || undefined}
            onChange={value => setExperimentID(String(value || ''))}
          />
        </div>
        <div>
          <Typography.Text strong>
            {I18n.t('smart_optimize_prompt_version', 'Prompt 版本')}
          </Typography.Text>
          <div className="mt-2">
            <Typography.Text code>{version || '-'}</Typography.Text>
          </div>
        </div>
      </div>
    </Modal>
  );
}
