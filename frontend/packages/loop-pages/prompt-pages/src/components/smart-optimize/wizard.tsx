// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable @coze-arch/max-line-per-function -- multi-step wizard UI */
/* eslint-disable max-lines-per-function -- multi-step wizard UI */
/* eslint-disable @typescript-eslint/no-magic-numbers -- mode/estimate defaults */
/* eslint-disable complexity -- step validation + submit */
import { useMemo, useState } from 'react';

import { ModelSelectWithObject } from '@cozeloop/prompt-components-v2';
import { I18n } from '@cozeloop/i18n-adapter';
import { ExperimentsSelect } from '@cozeloop/evaluate-components';
import { useModelList } from '@cozeloop/biz-hooks-adapter';
import { type Prompt } from '@cozeloop/api-schema/prompt';
import { type Model } from '@cozeloop/api-schema/llm-manage';
import { type ColumnEvalSetField } from '@cozeloop/api-schema/evaluation';
import {
  Button,
  Form,
  Input,
  Modal,
  Radio,
  Select,
  Slider,
  Toast,
  Typography,
} from '@coze-arch/coze-design';

import { type OptimizeSourceType } from './types';
import { ExperimentCaseSelector } from './experiment-case-selector';
import { optimizeTaskClient } from './client';

const MIN_CASES = 10;
const MAX_CASES = 500;

function findDefaultSourceField(
  variableKey: string,
  fields: ColumnEvalSetField[],
) {
  const exact = fields.find(
    field => field.key === variableKey || field.name === variableKey,
  );
  return exact?.key || fields[0]?.key || '';
}

export function SmartOptimizeWizard({
  visible,
  sourceType,
  prompt,
  spaceID,
  initialSource,
  embedded = false,
  onClose,
  onSubmitted,
}: {
  visible: boolean;
  sourceType: OptimizeSourceType;
  prompt?: Prompt;
  spaceID?: string;
  initialSource?: { id: string; name?: string };
  embedded?: boolean;
  onClose: () => void;
  onSubmitted?: (taskId: string) => void;
}) {
  const firstStep = initialSource ? 1 : 0;
  const [step, setStep] = useState(firstStep);
  const [sourceId, setSourceId] = useState('');
  const [sourceName, setSourceName] = useState('');
  const [selectedCaseIDs, setSelectedCaseIDs] = useState<string[]>([]);
  const [sourceFields, setSourceFields] = useState<ColumnEvalSetField[]>([]);
  const [variableMappings, setVariableMappings] = useState<
    Record<string, string>
  >({});
  const [modeScore, setModeScore] = useState(0.5);
  const [optimizerModel, setOptimizerModel] = useState<Model>();
  const [submitting, setSubmitting] = useState(false);
  const [variableField, setVariableField] = useState('input');
  const [actualField, setActualField] = useState('actual_output');
  const [referenceField, setReferenceField] = useState('reference_output');
  const modelService = useModelList(spaceID || '');

  const title = useMemo(
    () =>
      sourceType === 'experiment'
        ? I18n.t('smart_optimize_by_experiment', '基于评测实验优化 Prompt')
        : I18n.t('smart_optimize_by_eval_set', '基于优质的评测集优化 Prompt'),
    [sourceType],
  );
  const stepLabels = initialSource
    ? [
        I18n.t('smart_optimize_step_cases', '选择实验数据明细'),
        I18n.t('smart_optimize_step_mapping', '映射与优化模式'),
      ]
    : [
        I18n.t('smart_optimize_step_source', '选择数据源'),
        I18n.t('smart_optimize_step_cases', '选择实验数据明细'),
        I18n.t('smart_optimize_step_mapping', '映射与优化模式'),
      ];
  const activeStepIndex = step - firstStep;

  const estimate = useMemo(() => {
    const t = Math.round(2 + modeScore * 5);
    const n = Math.round(1 + modeScore * 3);
    const caseCount = Math.max(selectedCaseIDs.length, MIN_CASES);
    return Math.max(1, Math.round(caseCount * t * n * 0.02));
  }, [modeScore, selectedCaseIDs.length]);

  const reset = () => {
    setStep(firstStep);
    setSourceId('');
    setSourceName('');
    setSelectedCaseIDs([]);
    setSourceFields([]);
    setVariableMappings({});
    setModeScore(0.5);
    setOptimizerModel(undefined);
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
      return Boolean((sourceId || initialSource?.id)?.trim());
    }
    if (step === 1) {
      return (
        selectedCaseIDs.length >= MIN_CASES &&
        selectedCaseIDs.length <= MAX_CASES
      );
    }
    const variableDefs =
      prompt?.prompt_draft?.detail?.prompt_template?.variable_defs ??
      prompt?.prompt_commit?.detail?.prompt_template?.variable_defs ??
      [];
    return (
      Boolean(variableField && referenceField && optimizerModel?.model_id) &&
      variableDefs.every(variable =>
        Boolean(
          variableMappings[variable.key || ''] ||
            findDefaultSourceField(variable.key || '', sourceFields),
        ),
      )
    );
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
      const effectiveSourceId = (sourceId || initialSource?.id || '').trim();
      const effectiveSourceName = (
        sourceName ||
        initialSource?.name ||
        effectiveSourceId
      ).trim();
      const promptTemplate =
        prompt.prompt_draft?.detail?.prompt_template ??
        prompt.prompt_commit?.detail?.prompt_template;
      const variableDefs = promptTemplate?.variable_defs ?? [];

      const task = await optimizeTaskClient.createTask({
        workspace_id: spaceID,
        prompt_id: String(prompt.id),
        prompt_version: prompt.prompt_commit?.commit_info?.version,
        source:
          sourceType === 'experiment'
            ? {
                type: 'experiment',
                experiment_id: effectiveSourceId,
                experiment_name: effectiveSourceName,
              }
            : {
                type: 'eval_set',
                eval_set_id: effectiveSourceId,
                eval_set_version_id: effectiveSourceId,
                eval_set_name: effectiveSourceName,
              },
        case_item_ids: selectedCaseIDs,
        mapping: {
          variable_fields: variableDefs.length
            ? variableDefs.map(variable => ({
                field_name: variable.key || '',
                from_field_name:
                  variableMappings[variable.key || ''] ||
                  findDefaultSourceField(variable.key || '', sourceFields),
              }))
            : [{ field_name: 'input', from_field_name: variableField }],
          actual_output_field: actualField,
          reference_output_field: referenceField,
        },
        mode_score: modeScore,
        optimizer_model_id: String(optimizerModel?.model_id || ''),
        baseline_prompt: {
          prompt_id: String(prompt.id),
          prompt_version: prompt.prompt_commit?.commit_info?.version,
          messages: promptTemplate?.messages ?? [],
          variable_defs: promptTemplate?.variable_defs,
        },
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

  const footer = (
    <div className="flex justify-end gap-2">
      <Button onClick={handleClose}>{I18n.t('cancel', '取消')}</Button>
      {step > firstStep ? (
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
  );

  const content = (
    <>
      <div className="mb-4 flex gap-4 text-sm">
        {stepLabels.map((label, idx) => (
          <Typography.Text
            key={label}
            strong={idx === activeStepIndex}
            type={idx === activeStepIndex ? undefined : 'secondary'}
          >
            {idx + 1}. {label}
          </Typography.Text>
        ))}
      </div>

      {step === 0 ? (
        <div>
          <Form.Slot
            label={
              sourceType === 'experiment'
                ? I18n.t('smart_optimize_experiment_id', '实验 ID')
                : I18n.t('smart_optimize_eval_set_id', '评测集版本 ID')
            }
          >
            {sourceType === 'experiment' ? (
              <ExperimentsSelect
                value={sourceId || initialSource?.id || undefined}
                disableAddExperiment
                onChange={value => {
                  setSourceId(String(value || ''));
                  setSelectedCaseIDs([]);
                }}
              />
            ) : (
              <Input
                value={sourceId || initialSource?.id || ''}
                placeholder={I18n.t(
                  'smart_optimize_eval_set_placeholder',
                  '输入评测集版本 ID',
                )}
                onChange={v => setSourceId(String(v))}
              />
            )}
          </Form.Slot>
          <Form.Slot
            label={I18n.t('smart_optimize_source_name', '显示名称（可选）')}
          >
            <Input
              value={sourceName || initialSource?.name || ''}
              onChange={v => setSourceName(String(v))}
            />
          </Form.Slot>
          <Typography.Text type="secondary" size="small">
            {I18n.t(
              'smart_optimize_mock_hint',
              '实验列表与智能优化任务已接入真实服务。',
            )}
          </Typography.Text>
        </div>
      ) : null}

      {step === 1 ? (
        <div>
          <Typography.Text type="secondary" size="small">
            {I18n.t(
              'smart_optimize_case_rule',
              `请选择 ${MIN_CASES}-${MAX_CASES} 条实验数据。`,
            )}
          </Typography.Text>
          <div className="mt-3">
            <ExperimentCaseSelector
              spaceID={spaceID || ''}
              experimentID={sourceId || initialSource?.id || ''}
              selectedIDs={selectedCaseIDs}
              onSelectedIDsChange={setSelectedCaseIDs}
              onFieldsChange={setSourceFields}
            />
          </div>
        </div>
      ) : null}

      {step === 2 ? (
        <div>
          <Form.Slot
            label={I18n.t('smart_optimize_optimizer_model', '优化模型')}
          >
            <ModelSelectWithObject
              className="w-full"
              value={optimizerModel}
              modelList={modelService.data?.models ?? []}
              loading={modelService.loading}
              defaultSelectFirstModel
              onChange={setOptimizerModel}
            />
            <Typography.Text type="secondary" size="small">
              {I18n.t(
                'smart_optimize_optimizer_model_hint',
                '用于错误诊断、候选 Prompt 生成和无评估器时的结果裁判。',
              )}
            </Typography.Text>
          </Form.Slot>
          <Form.Slot
            label={I18n.t('smart_optimize_map_variable', 'Prompt 变量 ← 字段')}
          >
            {(
              prompt?.prompt_draft?.detail?.prompt_template?.variable_defs ??
              prompt?.prompt_commit?.detail?.prompt_template?.variable_defs ??
              []
            ).length ? (
              <div className="flex flex-col gap-2">
                {(
                  prompt?.prompt_draft?.detail?.prompt_template
                    ?.variable_defs ??
                  prompt?.prompt_commit?.detail?.prompt_template
                    ?.variable_defs ??
                  []
                ).map(variable => {
                  const key = variable.key || '';
                  return (
                    <div key={key} className="flex items-center gap-2">
                      <div className="w-40 truncate">{key}</div>
                      <span>←</span>
                      <Select
                        className="flex-1"
                        value={
                          variableMappings[key] ||
                          findDefaultSourceField(key, sourceFields)
                        }
                        optionList={sourceFields.map(field => ({
                          value: field.key || '',
                          label: field.name || field.key || '',
                        }))}
                        onChange={value =>
                          setVariableMappings(current => ({
                            ...current,
                            [key]: String(value || ''),
                          }))
                        }
                      />
                    </div>
                  );
                })}
              </div>
            ) : (
              <Input
                value={variableField}
                placeholder={sourceFields
                  .map(field => field.name || field.key)
                  .filter(Boolean)
                  .join(' / ')}
                onChange={v => setVariableField(String(v))}
              />
            )}
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
        </div>
      ) : null}
    </>
  );

  if (embedded) {
    return (
      <div className="flex h-full flex-col overflow-hidden bg-[var(--coz-bg-primary)]">
        <header className="flex h-16 shrink-0 items-center border-0 border-b border-solid border-[var(--coz-stroke-primary)] px-6">
          <Button type="tertiary" onClick={handleClose}>
            ← {I18n.t('smart_optimize_create', '新建智能优化')}
          </Button>
        </header>
        <main className="flex-1 overflow-auto px-8 py-6">
          <div className="mx-auto max-w-[1600px]">{content}</div>
        </main>
        <footer className="shrink-0 border-0 border-t border-solid border-[var(--coz-stroke-primary)] px-8 py-4">
          {footer}
        </footer>
      </div>
    );
  }

  return (
    <Modal
      title={title}
      visible={visible}
      onCancel={handleClose}
      width={step === 1 ? 1200 : 720}
      footer={footer}
    >
      {content}
    </Modal>
  );
}
