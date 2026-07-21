// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable @coze-arch/max-line-per-function -- composes Prompt workspace integrations */
import { useParams, useSearchParams } from 'react-router-dom';
import { useEffect, useMemo, useState } from 'react';

import {
  PromptDevelop,
  showSubmitSuccess,
} from '@cozeloop/prompt-components-v2';
import { I18n } from '@cozeloop/i18n-adapter';
import { useBreadcrumb, useModalData } from '@cozeloop/hooks';
import { useReportEvent } from '@cozeloop/components';
import {
  useModelList,
  useNavigateModule,
  useOpenWindow,
  useSpace,
} from '@cozeloop/biz-hooks-adapter';
import { uploadFile } from '@cozeloop/biz-components-adapter';
import { type Prompt } from '@cozeloop/api-schema/prompt';

import { TraceTab } from '@/components/trace-tabs';
import {
  renderSmartOptimizeHeaderButtons,
  SmartOptimizeCreateConfirmModal,
  SmartOptimizeTaskPanel,
} from '@/components/smart-optimize';
import { PromptWorkspacePanel } from '@/components/prompt-workspace-panel';
import { ExecuteHistoryPanel } from '@/components/execute-history-panel';

export default function PromptDevelopPage() {
  const sendEvent = useReportEvent();
  const { promptID } = useParams<{
    promptID: string;
  }>();
  const { spaceID } = useSpace();
  const [searchParams, setSearchParams] = useSearchParams();

  const [promptInfo, setPromptInfo] = useState<Prompt>();
  const [activeTab, setActiveTab] = useState(searchParams.get('tab') || 'dev');
  const [confirmVisible, setConfirmVisible] = useState(false);
  const [submitRequestKey, setSubmitRequestKey] = useState(0);
  const traceHistoryPannel = useModalData();
  const traceLogPannel = useModalData<string>();
  const navigate = useNavigateModule();
  const { openBlank } = useOpenWindow();

  useBreadcrumb({
    text: promptInfo?.prompt_basic?.display_name || '',
  });

  const service = useModelList(spaceID);

  useEffect(() => {
    const requestedTab = searchParams.get('tab');
    if (requestedTab) {
      setActiveTab(requestedTab);
    }
  }, [searchParams]);

  const extraTabs = useMemo(
    () => [
      {
        key: 'observation',
        title: I18n.t('observation', '观测'),
        children: (
          <PromptWorkspacePanel
            type="observation"
            promptKey={promptInfo?.prompt_key}
          />
        ),
      },
      {
        key: 'evaluation',
        title: I18n.t('evaluation', '评测'),
        children: <PromptWorkspacePanel type="evaluation" />,
      },
      {
        key: 'smart_optimize',
        title: I18n.t('smart_optimize', '智能优化'),
        children: (
          <SmartOptimizeTaskPanel
            promptID={promptID}
            spaceID={spaceID}
            onAdoptSuccess={() => {
              setActiveTab('dev');
              setSearchParams(current => {
                const next = new URLSearchParams(current);
                next.delete('tab');
                return next;
              });
              setSubmitRequestKey(value => value + 1);
            }}
          />
        ),
      },
    ],
    [promptID, promptInfo?.prompt_key, setSearchParams, spaceID],
  );

  const handleTabChange = (tabKey: string) => {
    setActiveTab(tabKey);
    setSearchParams(current => {
      const next = new URLSearchParams(current);
      if (tabKey === 'dev') {
        next.delete('tab');
      } else {
        next.set('tab', tabKey);
      }
      return next;
    });
  };

  return (
    <>
      <PromptDevelop
        bizID="CozeLoop"
        spaceID={spaceID}
        promptID={promptID}
        onPromptLoaded={setPromptInfo}
        modelInfo={{
          list: service.data?.models,
          loading: service.loading,
        }}
        sendEvent={sendEvent}
        multiModalConfig={{
          imageSupported: true,
          intranetUrlValidator: url => url.includes('localhost'),
        }}
        canDiffEdit={false}
        debugAreaConfig={{
          hideRoleChange: true,
          canEditMessageType: false,
        }}
        uploadFile={uploadFile}
        buttonConfig={{
          traceHistoryButton: {
            onClick: () => traceHistoryPannel.open(),
          },
          traceLogButton: {
            onClick: ({ debugId }) => traceLogPannel.open(debugId as string),
          },
          copyButton: {
            onSuccess: ({ prompt }) => openBlank(`pe/prompts/${prompt?.id}`),
          },
          deleteButton: {
            onSuccess: () => navigate('/pe/prompts'),
          },
        }}
        onSubmitSuccess={() => {
          showSubmitSuccess(
            () => navigate('observation/traces'),
            () => navigate('evaluation/datasets'),
          );
        }}
        hideSnippet={true}
        submitRequestKey={submitRequestKey}
        activeTab={activeTab}
        tabsChange={handleTabChange}
        extraTabs={extraTabs}
        renderHeaderButtons={buttons =>
          renderSmartOptimizeHeaderButtons(buttons, {
            onOpenWizard: sourceType => {
              if (sourceType === 'experiment') {
                setConfirmVisible(true);
                return;
              }
              navigate({
                pathname: `pe/prompts/${promptID}/optimize/create`,
                search: `?source=${sourceType}`,
              });
            },
          })
        }
      />
      <SmartOptimizeCreateConfirmModal
        visible={confirmVisible}
        prompt={promptInfo}
        onCancel={() => setConfirmVisible(false)}
        onConfirm={experimentID => {
          const version =
            promptInfo?.prompt_commit?.commit_info?.version ||
            promptInfo?.prompt_basic?.latest_version ||
            '';
          navigate({
            pathname: `pe/prompts/${promptID}/optimize/create`,
            search: `?source=experiment&experiment_id=${encodeURIComponent(
              experimentID,
            )}&prompt_version=${encodeURIComponent(version)}`,
          });
        }}
      />
      <TraceTab
        displayType="drawer"
        debugID={traceLogPannel.data}
        drawerVisible={traceLogPannel.visible}
        drawerClose={traceLogPannel.close}
      />
      <ExecuteHistoryPanel
        promptID={promptInfo?.id}
        visible={traceHistoryPannel.visible}
        onCancel={traceHistoryPannel.close}
      />
    </>
  );
}
