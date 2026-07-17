// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable @coze-arch/max-line-per-function -- task list + report modal */
/* eslint-disable @typescript-eslint/no-magic-numbers -- poll interval / score digits */
/* eslint-disable security/detect-object-injection -- status tag map lookup */
import { useCallback, useEffect, useState } from 'react';

import { usePromptStore } from '@cozeloop/prompt-components-v2';
import { I18n } from '@cozeloop/i18n-adapter';
import { Role } from '@cozeloop/api-schema/prompt';
import {
  Button,
  Modal,
  Progress,
  Table,
  Tag,
  Toast,
  Typography,
} from '@coze-arch/coze-design';

import { type OptimizeTask, type OptimizeTaskStatus } from './types';
import { mockOptimizeTaskClient } from './mock-client';

const STATUS_TAG: Record<OptimizeTaskStatus, { color: string; label: string }> =
  {
    queued: { color: 'grey', label: '排队中' },
    running: { color: 'blue', label: '运行中' },
    succeeded: { color: 'green', label: '成功' },
    failed: { color: 'red', label: '失败' },
    cancelled: { color: 'orange', label: '已终止' },
  };

function applyAdoptedPrompt(afterPrompt: string) {
  const { messageList, setMessageList } = usePromptStore.getState();
  const list = messageList || [];
  if (!list.length) {
    setMessageList([
      {
        key: 'adopt-system',
        role: Role.System,
        content: afterPrompt,
      },
    ]);
    return;
  }
  const systemIdx = list.findIndex(m => m.role === Role.System);
  const targetIdx = systemIdx >= 0 ? systemIdx : 0;
  setMessageList(
    list.map((m, i) =>
      i === targetIdx
        ? {
            ...m,
            content: afterPrompt,
          }
        : m,
    ),
  );
}

export function SmartOptimizeTaskPanel({
  promptID,
  spaceID,
  onAdoptSuccess,
}: {
  promptID?: string;
  spaceID?: string;
  onAdoptSuccess?: () => void;
}) {
  const [tasks, setTasks] = useState<OptimizeTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [reportTask, setReportTask] = useState<OptimizeTask | undefined>();

  const refresh = useCallback(async () => {
    if (!promptID || !spaceID) {
      return;
    }
    setLoading(true);
    try {
      const list = await mockOptimizeTaskClient.listTasks({
        workspace_id: spaceID,
        prompt_id: promptID,
      });
      setTasks(list);
    } finally {
      setLoading(false);
    }
  }, [promptID, spaceID]);

  useEffect(() => {
    void refresh();
    const timer = setInterval(() => {
      void refresh();
    }, 1500);
    return () => clearInterval(timer);
  }, [refresh]);

  const handleCancel = async (taskId: string) => {
    await mockOptimizeTaskClient.cancelTask(taskId);
    Toast.info(I18n.t('smart_optimize_cancelled', '已终止任务'));
    void refresh();
  };

  const handleAdopt = async (task: OptimizeTask) => {
    try {
      const { after_prompt } = await mockOptimizeTaskClient.adoptTask(task.id);
      applyAdoptedPrompt(after_prompt);
      Toast.success(
        I18n.t(
          'smart_optimize_adopted',
          '已写入草稿，请点击「提交新版」完成版本提交',
        ),
      );
      onAdoptSuccess?.();
      setReportTask(undefined);
    } catch (e) {
      Toast.error(
        e instanceof Error
          ? e.message
          : I18n.t('smart_optimize_adopt_failed', '采纳失败'),
      );
    }
  };

  return (
    <div className="h-full overflow-auto p-4">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <Typography.Title heading={5}>
            {I18n.t('smart_optimize_task_list', '智能优化任务')}
          </Typography.Title>
          <Typography.Text type="secondary" size="small">
            {I18n.t(
              'smart_optimize_task_list_hint',
              '当前使用本地 Mock API；后端 OptimizeTask 就绪后可无缝切换。',
            )}
          </Typography.Text>
        </div>
        <Button onClick={() => void refresh()} loading={loading}>
          {I18n.t('refresh', '刷新')}
        </Button>
      </div>

      <Table
        dataSource={tasks}
        rowKey="id"
        empty={
          <Typography.Text type="secondary">
            {I18n.t(
              'smart_optimize_empty',
              '暂无任务。可从 Header「智能优化」下拉发起。',
            )}
          </Typography.Text>
        }
        columns={[
          {
            title: I18n.t('smart_optimize_col_id', '任务 ID'),
            dataIndex: 'id',
            width: 140,
          },
          {
            title: I18n.t('smart_optimize_col_path', '路径'),
            dataIndex: 'source_type',
            width: 120,
            render: (v: OptimizeTask['source_type']) =>
              v === 'experiment'
                ? I18n.t('smart_optimize_path_experiment', '评测实验')
                : I18n.t('smart_optimize_path_eval_set', '优质评测集'),
          },
          {
            title: I18n.t('smart_optimize_col_source', '数据源'),
            dataIndex: 'source_name',
            render: (_: unknown, row: OptimizeTask) =>
              row.source_name || row.source_id,
          },
          {
            title: I18n.t('smart_optimize_col_status', '状态'),
            dataIndex: 'status',
            width: 160,
            render: (status: OptimizeTaskStatus, row: OptimizeTask) => {
              const meta = STATUS_TAG[status];
              return (
                <div className="flex flex-col gap-1">
                  <Tag color={meta.color as 'blue'} size="small">
                    {meta.label}
                  </Tag>
                  {status === 'running' || status === 'queued' ? (
                    <Progress percent={row.progress} size="small" />
                  ) : null}
                </div>
              );
            },
          },
          {
            title: I18n.t('smart_optimize_col_score', '得分'),
            width: 140,
            render: (_: unknown, row: OptimizeTask) =>
              row.result?.after_score
                ? `${row.result.before_score.toFixed(2)} → ${row.result.after_score.toFixed(2)}`
                : '-',
          },
          {
            title: I18n.t('smart_optimize_col_actions', '操作'),
            width: 200,
            render: (_: unknown, row: OptimizeTask) => (
              <div className="flex gap-2">
                {row.status === 'succeeded' ? (
                  <Button
                    size="small"
                    color="brand"
                    onClick={() => setReportTask(row)}
                  >
                    {I18n.t('smart_optimize_view_report', '报告')}
                  </Button>
                ) : null}
                {row.status === 'queued' || row.status === 'running' ? (
                  <Button
                    size="small"
                    type="tertiary"
                    onClick={() => void handleCancel(row.id)}
                  >
                    {I18n.t('terminate', '终止')}
                  </Button>
                ) : null}
              </div>
            ),
          },
        ]}
      />

      <Modal
        title={I18n.t('smart_optimize_report', '智能优化报告')}
        visible={Boolean(reportTask)}
        onCancel={() => setReportTask(undefined)}
        width={840}
        footer={
          <div className="flex justify-end gap-2">
            <Button onClick={() => setReportTask(undefined)}>
              {I18n.t('close', '关闭')}
            </Button>
            <Button
              color="brand"
              onClick={() => reportTask && void handleAdopt(reportTask)}
            >
              {I18n.t('smart_optimize_adopt', '采纳到草稿')}
            </Button>
          </div>
        }
      >
        {reportTask?.result ? (
          <div className="flex flex-col gap-4">
            <Typography.Text>
              {I18n.t('smart_optimize_score_delta', '综合得分')}：
              {reportTask.result.before_score.toFixed(2)} →{' '}
              {reportTask.result.after_score.toFixed(2)}
            </Typography.Text>
            {reportTask.result.diagnosis?.failure_modes?.length ? (
              <div>
                <Typography.Text strong>
                  {I18n.t('smart_optimize_diagnosis', '诊断')}
                </Typography.Text>
                <ul className="mt-1 list-disc pl-5">
                  {reportTask.result.diagnosis.failure_modes.map(item => (
                    <li key={item}>
                      <Typography.Text size="small">{item}</Typography.Text>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Typography.Text strong>
                  {I18n.t('smart_optimize_before', '优化前')}
                </Typography.Text>
                <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap rounded bg-[var(--coz-mg-primary)] p-3 text-xs">
                  {reportTask.result.before_prompt}
                </pre>
              </div>
              <div>
                <Typography.Text strong>
                  {I18n.t('smart_optimize_after', '优化后')}
                </Typography.Text>
                <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap rounded bg-[var(--coz-mg-primary)] p-3 text-xs">
                  {reportTask.result.after_prompt}
                </pre>
              </div>
            </div>
            <Table
              size="small"
              pagination={false}
              dataSource={reportTask.result.case_details}
              rowKey="case_id"
              columns={[
                { title: 'Case', dataIndex: 'case_id', width: 100 },
                {
                  title: I18n.t('smart_optimize_before_score', '优化前分'),
                  dataIndex: 'before_score',
                  width: 90,
                },
                {
                  title: I18n.t('smart_optimize_after_score', '优化后分'),
                  dataIndex: 'after_score',
                  width: 90,
                },
              ]}
            />
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
