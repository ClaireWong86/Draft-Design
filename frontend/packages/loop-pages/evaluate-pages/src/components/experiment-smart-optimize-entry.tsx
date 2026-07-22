// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable complexity -- experiment eligibility and launch orchestration */
import { useState } from 'react';

import {
  SmartOptimizeHeaderDropdown,
  type OptimizeSourceType,
  validatePromptForSmartOptimize,
} from '@cozeloop/prompt-pages';
import { I18n } from '@cozeloop/i18n-adapter';
import { useNavigateModule, useSpace } from '@cozeloop/biz-hooks-adapter';
import {
  EvalTargetType,
  ExptStatus,
  type Experiment,
} from '@cozeloop/api-schema/evaluation';
import { StonePromptApi } from '@cozeloop/api-schema';
import { Modal, Toast, Typography } from '@coze-arch/coze-design';

interface ExperimentSmartOptimizeEntryProps {
  experiment?: Experiment;
}

export function ExperimentSmartOptimizeEntry({
  experiment,
}: ExperimentSmartOptimizeEntryProps) {
  const { spaceID = '' } = useSpace();
  const navigate = useNavigateModule();
  const [confirmVisible, setConfirmVisible] = useState(false);
  const [loading, setLoading] = useState(false);

  const target = experiment?.eval_target;
  const targetVersion = target?.eval_target_version;
  const promptMeta = targetVersion?.eval_target_content?.prompt;
  const isPromptExperiment =
    target?.eval_target_type === EvalTargetType.CozeLoopPrompt;
  const canOptimize =
    experiment?.status === ExptStatus.Success && isPromptExperiment;

  const handleOpen = async (type: OptimizeSourceType) => {
    if (type === 'experiment' && !canOptimize) {
      Toast.warning(
        I18n.t(
          'smart_optimize_invalid_experiment',
          '仅支持已成功完成且评测对象为 Prompt 的实验',
        ),
      );
      return;
    }
    const promptID = promptMeta?.prompt_id || target?.source_target_id;
    if (!promptID) {
      Toast.error(
        I18n.t('smart_optimize_missing_prompt', '实验未关联可优化的 Prompt'),
      );
      return;
    }

    setLoading(true);
    try {
      const response = await StonePromptApi.GetPrompt({
        workspace_id: spaceID,
        prompt_id: promptID,
        with_commit: true,
        commit_version:
          promptMeta?.version || targetVersion?.source_target_version,
        with_draft: false,
      });
      if (!response.prompt) {
        throw new Error('Prompt not found');
      }
      const promptCheck = validatePromptForSmartOptimize(response.prompt);
      if (!promptCheck.ok) {
        Toast.warning(
          promptCheck.message ||
            I18n.t(
              'smart_optimize_prompt_invalid',
              '当前 Prompt 不满足智能优化条件',
            ),
        );
        return;
      }
      if (type === 'experiment') {
        setConfirmVisible(true);
      } else {
        navigate({
          pathname: `pe/prompts/${promptID}/optimize/create`,
          search: '?source=eval_set',
        });
      }
    } catch (error) {
      Toast.error(
        error instanceof Error
          ? error.message
          : I18n.t('smart_optimize_load_prompt_failed', '读取 Prompt 失败'),
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <div className={loading ? 'pointer-events-none opacity-60' : undefined}>
        <SmartOptimizeHeaderDropdown onOpenWizard={handleOpen} />
      </div>
      <Modal
        title={I18n.t('smart_optimize_create', '新建智能优化')}
        visible={confirmVisible}
        okText={I18n.t('confirm', '确定')}
        cancelText={I18n.t('cancel', '取消')}
        onCancel={() => setConfirmVisible(false)}
        onOk={() => {
          setConfirmVisible(false);
          const promptID = promptMeta?.prompt_id || target?.source_target_id;
          const promptVersion =
            promptMeta?.version || targetVersion?.source_target_version || '';
          navigate({
            pathname: `pe/prompts/${promptID}/optimize/create`,
            search: `?source=experiment&experiment_id=${encodeURIComponent(
              experiment?.id || '',
            )}&prompt_version=${encodeURIComponent(promptVersion)}`,
          });
        }}
        width={640}
      >
        <div className="mb-5 rounded-lg bg-[var(--coz-mg-hglt)] p-4">
          <Typography.Text>
            {I18n.t(
              'smart_optimize_experiment_intro',
              '基于批量评测的问题、模型回答、参考答案（如有）以及对应评估器得分来优化 Prompt',
            )}
          </Typography.Text>
        </div>
        <div className="flex flex-col gap-4">
          <div>
            <Typography.Text strong>
              {I18n.t('smart_optimize_selected_experiment', '选择评测实验')}
            </Typography.Text>
            <div className="mt-1">{experiment?.name || experiment?.id}</div>
          </div>
          <div>
            <Typography.Text strong>
              {I18n.t('smart_optimize_prompt_version', 'Prompt 版本')}
            </Typography.Text>
            <div className="mt-1">
              {promptMeta?.version ||
                targetVersion?.source_target_version ||
                '-'}
            </div>
          </div>
        </div>
      </Modal>
    </>
  );
}
