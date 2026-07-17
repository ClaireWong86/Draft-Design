// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable @coze-arch/max-line-per-function -- multi-step wizard UI */
/* eslint-disable max-lines-per-function -- multi-step wizard UI */
/* eslint-disable @typescript-eslint/no-magic-numbers -- mode/estimate defaults */
/* eslint-disable complexity -- step validation + submit */
import { useMemo, useState } from 'react';

import { I18n } from '@cozeloop/i18n-adapter';
import { type Prompt } from '@cozeloop/api-schema/prompt';
import {
  Button,
  Form,
  Input,
  Modal,
  Radio,
  Slider,
  Toast,
  Typography,
} from '@coze-arch/coze-design';

import { type OptimizeSourceType } from './types';
import { mockOptimizeTaskClient } from './mock-client';

const MIN_CASES = 10;
const MAX_CASES = 500;

export function SmartOptimizeWizard({
  visible,
  sourceType,
  prompt,
  spaceID,
  onClose,
  onSubmitted,
}: {
  visible: boolean;
  sourceType: OptimizeSourceType;
  prompt?: Prompt;
  spaceID?: string;
  onClose: () => void;
  onSubmitted?: (taskId: string) => void;
}) {
  const [step, setStep] = useState(0);
  const [sourceId, setSourceId] = useState('');
  const [sourceName, setSourceName] = useState('');
  const [caseCount, setCaseCount] = useState(20);
  const [modeScore, setModeScore] = useState(0.5);
  const [submitting, setSubmitting] = useState(false);
  const [variableField, setVariableField] = useState('input');
  const [actualField, setActualField] = useState('actual_output');
  const [referenceField, setReferenceField] = useState('reference_output');

  const title = useMemo(
    () =>
      sourceType === 'experiment'
        ? I18n.t('smart_optimize_by_experiment', '基于评测实验优化 Prompt')
        : I18n.t('smart_optimize_by_eval_set', '基于优质的评测集优化 Prompt'),
    [sourceType],
  );

  const estimate = useMemo(() => {
    const t = Math.round(2 + modeScore * 5);
    const n = Math.round(1 + modeScore * 3);
    return Math.max(1, Math.round(caseCount * t * n * 0.02));
  }, [caseCount, modeScore]);

  const reset = () => {
    setStep(0);
    setSourceId('');
    setSourceName('');
    setCaseCount(20);
    setModeScore(0.5);
    setVariableField('input');
    setActualField('actual_output');
    setReferenceField('reference_output');
  };

  const handleClose = () => {
    reset();
    onClose();
  };

  const canNext = () => {
    if (step === 0) {
      return Boolean(sourceId.trim());
    }
    if (step === 1) {
      return caseCount >= MIN_CASES && caseCount <= MAX_CASES;
    }
    return Boolean(variableField && referenceField);
  };

  const handleSubmit = async () => {
    if (!spaceID || !prompt?.id) {
      Toast.error(
        I18n.t('smart_optimize_missing_context', '缺少空间或 Prompt 上下文'),
      );
      return;
    }
    setSubmitting(true);
    try {
      const caseIds = Array.from(
        { length: caseCount },
        (_, i) => `mock-case-${i + 1}`,
      );
      const baseline =
        prompt.prompt_draft?.detail?.prompt_template?.messages
          ?.map(m => m.content || '')
          .join('\n\n') ||
        prompt.prompt_commit?.detail?.prompt_template?.messages
          ?.map(m => m.content || '')
          .join('\n\n') ||
        '';

      const task = await mockOptimizeTaskClient.createTask({
        workspace_id: spaceID,
        prompt_id: String(prompt.id),
        prompt_version: prompt.prompt_commit?.commit_info?.version,
        source_type: sourceType,
        source_id: sourceId.trim(),
        source_name: sourceName.trim() || sourceId.trim(),
        case_item_ids: caseIds,
        mapping: {
          variable_fields: [
            { field_name: 'input', from_field_name: variableField },
          ],
          actual_output_field: actualField,
          reference_output_field: referenceField,
        },
        mode_score: modeScore,
        baseline_prompt: baseline,
      });
      Toast.success(
        I18n.t('smart_optimize_task_created', '优化任务已创建（Mock）'),
      );
      onSubmitted?.(task.id);
      handleClose();
    } catch (e) {
      Toast.error(
        e instanceof Error
          ? e.message
          : I18n.t('smart_optimize_task_create_failed', '创建任务失败'),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      title={title}
      visible={visible}
      onCancel={handleClose}
      width={720}
      footer={
        <div className="flex justify-end gap-2">
          <Button onClick={handleClose}>{I18n.t('cancel', '取消')}</Button>
          {step > 0 ? (
            <Button onClick={() => setStep(s => s - 1)}>
              {I18n.t('smart_optimize_prev', '上一步')}
            </Button>
          ) : null}
          {step < 2 ? (
            <Button
              color="brand"
              disabled={!canNext()}
              onClick={() => setStep(s => s + 1)}
            >
              {I18n.t('smart_optimize_next', '下一步')}
            </Button>
          ) : (
            <Button
              color="brand"
              loading={submitting}
              disabled={!canNext()}
              onClick={handleSubmit}
            >
              {I18n.t('smart_optimize_submit', '开始优化')}
            </Button>
          )}
        </div>
      }
    >
      <div className="mb-4 flex gap-4 text-sm">
        {[
          I18n.t('smart_optimize_step_source', '选择数据源'),
          I18n.t('smart_optimize_step_cases', '选择样本'),
          I18n.t('smart_optimize_step_mapping', '映射与模式'),
        ].map((label, idx) => (
          <Typography.Text
            key={label}
            strong={idx === step}
            type={idx === step ? undefined : 'secondary'}
          >
            {idx + 1}. {label}
          </Typography.Text>
        ))}
      </div>

      {step === 0 ? (
        <Form layout="vertical">
          <Form.Slot
            label={
              sourceType === 'experiment'
                ? I18n.t('smart_optimize_experiment_id', '实验 ID')
                : I18n.t('smart_optimize_eval_set_id', '评测集版本 ID')
            }
          >
            <Input
              value={sourceId}
              placeholder={
                sourceType === 'experiment'
                  ? I18n.t(
                      'smart_optimize_experiment_placeholder',
                      '输入已成功实验 ID（Mock 阶段任意非空即可）',
                    )
                  : I18n.t(
                      'smart_optimize_eval_set_placeholder',
                      '输入评测集版本 ID（Mock 阶段任意非空即可）',
                    )
              }
              onChange={v => setSourceId(String(v))}
            />
          </Form.Slot>
          <Form.Slot
            label={I18n.t('smart_optimize_source_name', '显示名称（可选）')}
          >
            <Input
              value={sourceName}
              onChange={v => setSourceName(String(v))}
            />
          </Form.Slot>
          <Typography.Text type="secondary" size="small">
            {I18n.t(
              'smart_optimize_mock_hint',
              '当前为 Mock 阶段：不校验真实实验/评测集，提交后在「智能优化」Tab 查看任务进度。',
            )}
          </Typography.Text>
        </Form>
      ) : null}

      {step === 1 ? (
        <Form layout="vertical">
          <Form.Slot
            label={I18n.t(
              'smart_optimize_case_count',
              `样本数量（${MIN_CASES}-${MAX_CASES}）`,
            )}
          >
            <Input
              type="number"
              value={String(caseCount)}
              onChange={v => setCaseCount(Number(v) || 0)}
            />
          </Form.Slot>
          <Typography.Text type="secondary" size="small">
            {I18n.t(
              'smart_optimize_case_mock_note',
              'Mock 将自动生成对应数量的虚拟 case ID；联调后将替换为真实样本选择表。',
            )}
          </Typography.Text>
        </Form>
      ) : null}

      {step === 2 ? (
        <Form layout="vertical">
          <Form.Slot
            label={I18n.t('smart_optimize_map_variable', 'Prompt 变量 ← 字段')}
          >
            <Input
              value={variableField}
              onChange={v => setVariableField(String(v))}
            />
          </Form.Slot>
          <Form.Slot
            label={I18n.t(
              'smart_optimize_map_actual',
              '模型回答 actual_output ← 字段',
            )}
          >
            <Input
              value={actualField}
              onChange={v => setActualField(String(v))}
            />
          </Form.Slot>
          <Form.Slot
            label={I18n.t(
              'smart_optimize_map_reference',
              '参考回答 reference_output ← 字段',
            )}
          >
            <Input
              value={referenceField}
              onChange={v => setReferenceField(String(v))}
            />
          </Form.Slot>
          <Form.Slot label={I18n.t('smart_optimize_mode', '优化模式')}>
            <div className="px-1">
              <div className="mb-2 flex justify-between">
                <Typography.Text size="small">
                  {I18n.t('smart_optimize_mode_cost', '性价比优先')}
                </Typography.Text>
                <Typography.Text size="small">
                  {I18n.t('smart_optimize_mode_quality', '效果优先')}
                </Typography.Text>
              </div>
              <Slider
                min={0}
                max={1}
                step={0.1}
                value={modeScore}
                onChange={v => setModeScore(Number(v))}
              />
            </div>
          </Form.Slot>
          <Form.Slot label={I18n.t('smart_optimize_mode_preset', '快捷预设')}>
            <Radio.Group
              value={
                modeScore <= 0.3
                  ? 'cost'
                  : modeScore >= 0.7
                    ? 'quality'
                    : 'balanced'
              }
              onChange={e => {
                const v = e.target.value;
                if (v === 'cost') {
                  setModeScore(0.2);
                } else if (v === 'quality') {
                  setModeScore(0.8);
                } else {
                  setModeScore(0.5);
                }
              }}
            >
              <Radio value="cost">
                {I18n.t('smart_optimize_mode_cost', '性价比优先')}
              </Radio>
              <Radio value="balanced">
                {I18n.t('smart_optimize_mode_balanced', '均衡')}
              </Radio>
              <Radio value="quality">
                {I18n.t('smart_optimize_mode_quality', '效果优先')}
              </Radio>
            </Radio.Group>
          </Form.Slot>
          <Typography.Text type="secondary">
            {I18n.t(
              'smart_optimize_estimate',
              `预估消耗约 ${estimate} 资源点（Mock 估算，仅供参考）`,
            )}
          </Typography.Text>
        </Form>
      ) : null}
    </Modal>
  );
}
