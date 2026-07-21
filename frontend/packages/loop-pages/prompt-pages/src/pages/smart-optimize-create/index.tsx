// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { useParams, useSearchParams } from 'react-router-dom';

import { useRequest } from 'ahooks';
import { I18n } from '@cozeloop/i18n-adapter';
import { useNavigateModule, useSpace } from '@cozeloop/biz-hooks-adapter';
import { StoneEvaluationApi, StonePromptApi } from '@cozeloop/api-schema';
import { Spin, Typography } from '@coze-arch/coze-design';

import {
  SmartOptimizeWizard,
  type OptimizeSourceType,
} from '@/components/smart-optimize';

export default function SmartOptimizeCreatePage() {
  const { promptID = '' } = useParams<{ promptID: string }>();
  const [searchParams] = useSearchParams();
  const { spaceID = '' } = useSpace();
  const navigate = useNavigateModule();
  const sourceType = (searchParams.get('source') ||
    'experiment') as OptimizeSourceType;
  const sourceID =
    sourceType === 'experiment'
      ? searchParams.get('experiment_id') || ''
      : searchParams.get('eval_set_version_id') || '';
  const evalSetID = searchParams.get('eval_set_id') || '';
  const promptVersion = searchParams.get('prompt_version') || undefined;

  const service = useRequest(
    async () => {
      const [promptResponse, experimentResponse] = await Promise.all([
        StonePromptApi.GetPrompt({
          workspace_id: spaceID,
          prompt_id: promptID,
          with_commit: true,
          commit_version: promptVersion,
          with_draft: false,
        }),
        sourceType === 'experiment' && sourceID
          ? StoneEvaluationApi.BatchGetExperiments({
              workspace_id: spaceID,
              expt_ids: [sourceID],
            })
          : Promise.resolve(undefined),
      ]);
      return {
        prompt: promptResponse.prompt,
        sourceName: experimentResponse?.experiments?.[0]?.name,
      };
    },
    { ready: Boolean(spaceID && promptID) },
  );

  const returnToTaskList = () => {
    navigate({
      pathname: `pe/prompts/${promptID}`,
      search: '?tab=smart_optimize',
    });
  };

  if (service.loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spin spinning />
      </div>
    );
  }

  if (!service.data?.prompt) {
    return (
      <div className="flex h-full items-center justify-center">
        <Typography.Text type="secondary">
          {I18n.t(
            'smart_optimize_prompt_load_failed',
            '未能加载待优化的 Prompt',
          )}
        </Typography.Text>
      </div>
    );
  }

  return (
    <SmartOptimizeWizard
      embedded
      visible
      sourceType={sourceType}
      prompt={service.data.prompt}
      spaceID={spaceID}
      initialSource={
        sourceID
          ? {
              id: sourceID,
              parentId: sourceType === 'eval_set' ? evalSetID : undefined,
              name: service.data.sourceName,
            }
          : undefined
      }
      onClose={returnToTaskList}
      onSubmitted={returnToTaskList}
    />
  );
}
