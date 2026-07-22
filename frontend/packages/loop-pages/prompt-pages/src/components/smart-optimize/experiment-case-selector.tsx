// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable complexity -- multimodal content rendering */
import { useEffect, useMemo, useState } from 'react';

import { useRequest } from 'ahooks';
import { I18n } from '@cozeloop/i18n-adapter';
import {
  ContentType,
  TurnRunState,
  type ColumnEvalSetField,
  type Content,
  type ItemResult,
} from '@cozeloop/api-schema/evaluation';
import { StoneEvaluationApi } from '@cozeloop/api-schema';
import {
  Button,
  Checkbox,
  Pagination,
  Select,
  Spin,
  Tag,
  Typography,
} from '@coze-arch/coze-design';

const PAGE_SIZE = 20;

interface ExperimentCaseSelectorProps {
  spaceID: string;
  experimentID: string;
  selectedIDs: string[];
  onSelectedIDsChange: (ids: string[]) => void;
  onFieldsChange?: (fields: ColumnEvalSetField[]) => void;
}

function contentToText(content?: Content): string {
  if (!content) {
    return '-';
  }
  if (content.content_type === ContentType.MultiPart) {
    return (content.multi_part ?? []).map(contentToText).join(' ');
  }
  if (content.content_type === ContentType.Image) {
    return content.image?.name || '[图片]';
  }
  if (content.content_type === ContentType.Video) {
    return content.video?.name || '[视频]';
  }
  return content.text || '-';
}

function ContentPreview({ content }: { content?: Content }) {
  if (content?.content_type === ContentType.Image) {
    const src = content.image?.thumb_url || content.image?.url;
    return src ? (
      <img
        src={src}
        alt={content.image?.name || 'evaluation input'}
        className="h-12 w-12 rounded object-cover"
      />
    ) : (
      <span>[图片]</span>
    );
  }
  if (content?.content_type === ContentType.MultiPart) {
    return (
      <div className="flex max-w-[260px] flex-wrap gap-1">
        {(content.multi_part ?? []).map((part, index) => (
          <ContentPreview
            key={`${part.content_type}-${index}`}
            content={part}
          />
        ))}
      </div>
    );
  }
  return (
    <Typography.Text ellipsis={{ showTooltip: true }} className="max-w-[260px]">
      {contentToText(content)}
    </Typography.Text>
  );
}

function getRow(item: ItemResult, experimentID: string) {
  const turn = item.turn_results?.[0];
  const result = turn?.experiment_results?.find(
    experiment => experiment.experiment_id === experimentID,
  );
  const payload = result?.payload;
  const fields = payload?.eval_set?.turn?.field_data_list ?? [];
  const reference = fields.find(field =>
    /reference|expected|answer|label/i.test(`${field.key} ${field.name}`),
  );
  const input = fields.find(field => field !== reference);
  const actual =
    payload?.target_output?.eval_target_record?.eval_target_output_data
      ?.output_fields?.actual_output;
  const evaluatorScores = Object.values(
    payload?.evaluator_output?.evaluator_records ?? {},
  ).map(record => record.evaluator_output_data?.evaluator_result?.score);

  return {
    id: `${item.item_id}_${turn?.turn_id || '0'}`,
    input: input?.content,
    reference: reference?.content,
    actual,
    status: payload?.system_info?.turn_run_state,
    evaluatorScores,
  };
}

type CaseStatus = 'all' | 'success' | 'failed';

function SelectionToolbar({
  status,
  selectedCount,
  onStatusChange,
  onClear,
}: {
  status: CaseStatus;
  selectedCount: number;
  onStatusChange: (status: CaseStatus) => void;
  onClear: () => void;
}) {
  return (
    <div className="flex items-center gap-3">
      <Select
        value={status}
        className="w-36"
        optionList={[
          { value: 'all', label: I18n.t('all', '全部状态') },
          { value: 'success', label: I18n.t('success', '成功') },
          { value: 'failed', label: I18n.t('failure', '失败') },
        ]}
        onChange={value => onStatusChange(value as CaseStatus)}
      />
      <Typography.Text type="secondary">
        {I18n.t('smart_optimize_selected_count', '已选')} {selectedCount}{' '}
        {I18n.t('item_unit', '条数据')}
      </Typography.Text>
      {selectedCount ? (
        <Button type="tertiary" size="small" onClick={onClear}>
          {I18n.t('cancel_select', '取消选择')}
        </Button>
      ) : null}
    </div>
  );
}

export function ExperimentCaseSelector({
  spaceID,
  experimentID,
  selectedIDs,
  onSelectedIDsChange,
  onFieldsChange,
}: ExperimentCaseSelectorProps) {
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState<CaseStatus>('all');
  const service = useRequest(
    () =>
      StoneEvaluationApi.BatchGetExperimentResult({
        workspace_id: spaceID,
        experiment_ids: [experimentID],
        baseline_experiment_id: experimentID,
        page_number: page,
        page_size: PAGE_SIZE,
        use_accelerator: true,
      }),
    {
      ready: Boolean(spaceID && experimentID),
      refreshDeps: [spaceID, experimentID, page],
    },
  );

  useEffect(() => {
    onFieldsChange?.(service.data?.column_eval_set_fields ?? []);
  }, [onFieldsChange, service.data?.column_eval_set_fields]);

  const rows = useMemo(
    () =>
      (service.data?.item_results ?? [])
        .map(item => getRow(item, experimentID))
        .filter(row => {
          if (status === 'success') {
            return row.status === TurnRunState.Success;
          }
          if (status === 'failed') {
            return row.status === TurnRunState.Fail;
          }
          return true;
        }),
    [experimentID, service.data?.item_results, status],
  );
  const pageIDs = rows.map(row => row.id);
  const allPageSelected =
    pageIDs.length > 0 && pageIDs.every(id => selectedIDs.includes(id));

  const toggle = (id: string, checked: boolean) => {
    onSelectedIDsChange(
      checked
        ? Array.from(new Set([...selectedIDs, id]))
        : selectedIDs.filter(selectedID => selectedID !== id),
    );
  };

  return (
    <div className="flex min-h-[420px] flex-col gap-3">
      <SelectionToolbar
        status={status}
        selectedCount={selectedIDs.length}
        onStatusChange={setStatus}
        onClear={() => onSelectedIDsChange([])}
      />
      <Spin spinning={service.loading}>
        <div className="overflow-auto rounded-lg border border-solid border-[var(--coz-stroke-primary)]">
          <table className="w-full table-fixed border-collapse text-left">
            <thead className="bg-[var(--coz-mg-secondary)]">
              <tr>
                <th className="w-12 p-3">
                  <Checkbox
                    checked={allPageSelected}
                    onChange={event => {
                      const checked = Boolean(event.target.checked);
                      onSelectedIDsChange(
                        checked
                          ? Array.from(new Set([...selectedIDs, ...pageIDs]))
                          : selectedIDs.filter(id => !pageIDs.includes(id)),
                      );
                    }}
                  />
                </th>
                <th className="w-32 p-3">ID</th>
                <th className="p-3">input</th>
                <th className="p-3">reference_output</th>
                <th className="p-3">actual_output</th>
                <th className="w-24 p-3">{I18n.t('status', '状态')}</th>
                <th className="w-32 p-3">{I18n.t('score', '评估得分')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(row => (
                <tr
                  key={row.id}
                  className="border-0 border-t border-solid border-[var(--coz-stroke-primary)]"
                >
                  <td className="p-3">
                    <Checkbox
                      checked={selectedIDs.includes(row.id)}
                      onChange={event =>
                        toggle(row.id, Boolean(event.target.checked))
                      }
                    />
                  </td>
                  <td className="truncate p-3">{row.id}</td>
                  <td className="p-3">
                    <ContentPreview content={row.input} />
                  </td>
                  <td className="p-3">
                    <ContentPreview content={row.reference} />
                  </td>
                  <td className="p-3">
                    <ContentPreview content={row.actual} />
                  </td>
                  <td className="p-3">
                    <Tag
                      color={
                        row.status === TurnRunState.Success ? 'green' : 'red'
                      }
                    >
                      {row.status === TurnRunState.Success
                        ? I18n.t('success', '成功')
                        : I18n.t('failure', '失败')}
                    </Tag>
                  </td>
                  <td className="p-3">
                    {row.evaluatorScores
                      .filter(score => score !== undefined)
                      .map(score => Number(score).toFixed(2))
                      .join(' / ') || '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Spin>
      <div className="flex justify-end">
        <Pagination
          currentPage={page}
          pageSize={PAGE_SIZE}
          total={service.data?.total ?? 0}
          onPageChange={setPage}
        />
      </div>
    </div>
  );
}
