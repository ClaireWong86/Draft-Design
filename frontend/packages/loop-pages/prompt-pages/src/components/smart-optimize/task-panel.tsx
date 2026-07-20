// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable @coze-arch/max-line-per-function -- task list + report modal */
/* eslint-disable @typescript-eslint/no-magic-numbers -- poll interval / score digits */
/* eslint-disable security/detect-object-injection -- status tag map lookup */
import { useCallback, useEffect, useMemo, useState } from 'react';

import { usePromptStore } from '@cozeloop/prompt-components-v2';
import { I18n } from '@cozeloop/i18n-adapter';
import {
  Button,
  Input,
  Modal,
  Progress,
  Select,
  Table,
  Tag,
  Toast,
  Typography,
} from '@coze-arch/coze-design';

import {
  type OptimizePromptSnapshot,
  type OptimizeTask,
  type OptimizeTaskStatus,
} from './types';
import { optimizeTaskClient } from './client';

const STATUS_TAG: Record<OptimizeTaskStatus, { color: string; label: string }> =
  {
    queued: { color: 'grey', label: '排队中' },
    running: { color: 'blue', label: '运行中' },
    succeeded: { color: 'green', label: '成功' },
    failed: { color: 'red', label: '失败' },
    cancelled: { color: 'orange', label: '已终止' },
  };

function applyAdoptedPrompt(afterPrompt: OptimizePromptSnapshot) {
  const { setMessageList } = usePromptStore.getState();
  setMessageList(
    afterPrompt.messages.map((message, index) => ({
      ...message,
      key: message.metadata?.key || `optimized-message-${index}`,
    })),
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
  const [keyword, setKeyword] = useState('');
  const [filterStatus, setFilterStatus] = useState<OptimizeTaskStatus | 'all'>(
    'all',
  );

  const filteredTasks = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    return tasks.filter(task => {
      const sourceName =
        task.source.type === 'experiment'
          ? task.source.experiment_name || task.source.experiment_id
          : task.source.eval_set_name || task.source.eval_set_version_id;
      const matchesKeyword =
        !normalizedKeyword ||
        (task.name || task.id).toLowerCase().includes(normalizedKeyword) ||
        sourceName.toLowerCase().includes(normalizedKeyword);
      return (
        matchesKeyword &&
        (filterStatus === 'all' || task.status === filterStatus)
      );
    });
  }, [filterStatus, keyword, tasks]);

  const refresh = useCallback(async () => {
    if (!promptID || !spaceID) {
      return;
    }
    setLoading(true);
    try {
      const list = await optimizeTaskClient.listTasks({
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
    if (!spaceID) {
      return;
    }
    await optimizeTaskClient.cancelTask(taskId, spaceID);
    Toast.info(I18n.t('smart_optimize_cancelled', '已终止任务'));
    void refresh();
  };

  const handleAdopt = async (task: OptimizeTask) => {
    try {
      if (!spaceID) {
        throw new Error('缺少空间上下文');
      }
      const { after_prompt } = await optimizeTaskClient.adoptTask(
        task.id,
        spaceID,
      );
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
      <div className="mb-4 flex items-start justify-between">
        <div>
          <Typography.Title heading={5}>
            {I18n.t('smart_optimize_task_list', '智能优化任务')}
          </Typography.Title>
          <Typography.Text type="secondary" size="small">
            {I18n.t(
              'smart_optimize_task_list_hint',
              '任务由后端异步执行；完成后可查看诊断并采纳优化 Prompt。',
            )}
          </Typography.Text>
        </div>
        <Button onClick={() => void refresh()} loading={loading}>
          {I18n.t('refresh', '刷新')}
        </Button>
      </div>

      <div className="mb-4 flex items-center gap-3">
        <Input
          className="w-80"
          value={keyword}
          placeholder={I18n.t('smart_optimize_search_name', '搜索任务名称')}
          onChange={value => setKeyword(String(value))}
        />
        <Select
          className="w-48"
          value={filterStatus}
          optionList={[
            { value: 'all', label: I18n.t('all_status', '全部状态') },
            ...Object.entries(STATUS_TAG).map(([value, meta]) => ({
              value,
              label: meta.label,
            })),
          ]}
          onChange={value =>
            setFilterStatus((value || 'all') as OptimizeTaskStatus | 'all')
          }
        />
        <Typography.Text type="secondary" size="small">
          {I18n.t(
            'smart_optimize_task_count',
            `共 ${filteredTasks.length} 个任务`,
          )}
        </Typography.Text>
      </div>

      <Table
        empty={
          <Typography.Text type="secondary">
            {I18n.t(
              'smart_optimize_empty',
              '暂无任务。可从 Header「智能优化」下拉发起。',
            )}
          </Typography.Text>
        }
        tableProps={{
          dataSource: filteredTasks,
          rowKey: 'id',
          columns: [
            {
              title: I18n.t('smart_optimize_col_name', '任务名称'),
              dataIndex: 'name',
              width: 220,
              render: (name: string | undefined, row: OptimizeTask) =>
                name || row.id,
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
              title: I18n.t('smart_optimize_col_case_count', '数据条数'),
              width: 100,
              render: (_: unknown, row: OptimizeTask) =>
                row.case_item_ids.length,
            },
            {
              title: I18n.t('smart_optimize_col_mode', '优化模式'),
              width: 120,
              render: (_: unknown, row: OptimizeTask) =>
                row.mode_score >= 0.7
                  ? I18n.t('smart_optimize_mode_quality', '效果优先')
                  : row.mode_score <= 0.3
                    ? I18n.t('smart_optimize_mode_cost', '性价比优先')
                    : I18n.t('smart_optimize_mode_balanced', '均衡'),
            },
            {
              title: I18n.t('smart_optimize_col_result', '优化结果'),
              width: 140,
              render: (_: unknown, row: OptimizeTask) =>
                row.status === 'succeeded' &&
                row.result &&
                typeof row.result.before_score === 'number' &&
                typeof row.result.after_score === 'number'
                  ? `${row.result.before_score.toFixed(2)} → ${row.result.after_score.toFixed(2)}`
                  : row.status === 'succeeded'
                    ? I18n.t(
                        'smart_optimize_candidate_ready',
                        '候选已生成（待重评）',
                      )
                    : row.status === 'failed'
                      ? row.error_msg || I18n.t('smart_optimize_failed', '失败')
                      : '-',
            },
            {
              title: I18n.t('smart_optimize_col_eval_set', '关联评测集'),
              width: 160,
              render: (_: unknown, row: OptimizeTask) =>
                row.source.type === 'eval_set'
                  ? row.source.eval_set_name || row.source.eval_set_version_id
                  : '-',
            },
            {
              title: I18n.t('smart_optimize_col_experiment', '关联评测实验'),
              width: 180,
              render: (_: unknown, row: OptimizeTask) =>
                row.source.type === 'experiment'
                  ? row.source.experiment_name || row.source.experiment_id
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
          ],
        }}
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
            {typeof reportTask.result.before_score === 'number' &&
            typeof reportTask.result.after_score === 'number' ? (
              <Typography.Text>
                {I18n.t('smart_optimize_score_delta', '综合得分')}：
                {reportTask.result.before_score.toFixed(2)} →{' '}
                {reportTask.result.after_score.toFixed(2)}
              </Typography.Text>
            ) : (
              <Typography.Text type="secondary">
                {I18n.t(
                  'smart_optimize_score_pending',
                  '优化候选已生成，需通过评测实验完成效果重评。',
                )}
              </Typography.Text>
            )}
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
                  {reportTask.result.before_prompt.messages
                    .map(
                      message => `[${message.role}] ${message.content || ''}`,
                    )
                    .join('\n\n')}
                </pre>
              </div>
              <div>
                <Typography.Text strong>
                  {I18n.t('smart_optimize_after', '优化后')}
                </Typography.Text>
                <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap rounded bg-[var(--coz-mg-primary)] p-3 text-xs">
                  {reportTask.result.after_prompt.messages
                    .map(
                      message => `[${message.role}] ${message.content || ''}`,
                    )
                    .join('\n\n')}
                </pre>
              </div>
            </div>
            <Table
              tableProps={{
                size: 'small',
                pagination: false,
                dataSource: reportTask.result.case_details,
                rowKey: 'case_id',
                columns: [
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
                ],
              }}
            />
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
