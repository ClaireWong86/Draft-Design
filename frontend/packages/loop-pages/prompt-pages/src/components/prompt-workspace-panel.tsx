// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { I18n } from '@cozeloop/i18n-adapter';
import { useNavigateModule } from '@cozeloop/biz-hooks-adapter';
import { Button, Typography } from '@coze-arch/coze-design';

export function PromptWorkspacePanel({
  type,
  promptKey,
}: {
  type: 'observation' | 'evaluation';
  promptKey?: string;
}) {
  const navigate = useNavigateModule();
  const isObservation = type === 'observation';

  const openTraces = () => {
    if (!promptKey) {
      return;
    }
    const filter = encodeURIComponent(
      encodeURIComponent(
        JSON.stringify({
          query_and_or: 'and',
          filter_fields: [
            {
              field_name: 'prompt_key',
              logic_field_name_type: 'prompt_key',
              query_type: 'in',
              values: [promptKey],
            },
          ],
        }),
      ),
    );
    navigate(
      `observation/traces?relation=and&selected_span_type=root_span&trace_filters=${filter}&trace_platform=prompt`,
    );
  };

  return (
    <div className="flex h-full items-center justify-center bg-[#FCFCFF] p-8">
      <div className="w-full max-w-2xl rounded-xl border border-solid border-[var(--coz-stroke-primary)] bg-white p-8 text-center">
        <Typography.Title heading={4}>
          {isObservation
            ? I18n.t('prompt_observation', 'Prompt 观测')
            : I18n.t('prompt_evaluation', 'Prompt 评测')}
        </Typography.Title>
        <Typography.Paragraph type="secondary" className="mt-3">
          {isObservation
            ? I18n.t(
                'prompt_observation_hint',
                '查看与当前 Prompt Key 关联的 Trace、输入输出、Token 与耗时。',
              )
            : I18n.t(
                'prompt_evaluation_hint',
                '使用评测集、评估器和实验验证当前 Prompt 版本，为智能优化提供证据。',
              )}
        </Typography.Paragraph>
        <div className="mt-6 flex justify-center gap-3">
          {isObservation ? (
            <Button color="brand" disabled={!promptKey} onClick={openTraces}>
              {I18n.t('prompt_view_related_traces', '查看关联 Trace')}
            </Button>
          ) : (
            <>
              <Button
                color="brand"
                onClick={() => navigate('evaluation/experiments')}
              >
                {I18n.t('prompt_view_experiments', '查看评测实验')}
              </Button>
              <Button onClick={() => navigate('evaluation/datasets')}>
                {I18n.t('prompt_view_eval_sets', '查看评测集')}
              </Button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
