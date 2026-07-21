// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useMemo, useState } from 'react';

import { useRequest } from 'ahooks';
import { I18n } from '@cozeloop/i18n-adapter';
import {
  ContentType,
  type ColumnEvalSetField,
  type Content,
} from '@cozeloop/api-schema/evaluation';
import { StoneEvaluationApi } from '@cozeloop/api-schema';
import { Checkbox, Pagination, Spin, Typography } from '@coze-arch/coze-design';

const PAGE_SIZE = 20;

function ContentPreview({ content }: { content?: Content }) {
  if (content?.content_type === ContentType.Image) {
    const src = content.image?.thumb_url || content.image?.url;
    return src ? (
      <img src={src} alt={content.image?.name || 'evaluation input'} className="h-12 w-12 rounded object-cover" />
    ) : <span>[图片]</span>;
  }
  if (content?.content_type === ContentType.MultiPart) {
    return <div className="flex max-w-[280px] flex-wrap gap-1">{(content.multi_part ?? []).map((part, index) => <ContentPreview key={`${part.content_type}-${index}`} content={part} />)}</div>;
  }
  return <Typography.Text ellipsis={{ showTooltip: true }} className="max-w-[280px]">{content?.text || '-'}</Typography.Text>;
}

export function EvaluationSetCaseSelector({
  spaceID,
  evaluationSetID,
  versionID,
  selectedIDs,
  onSelectedIDsChange,
  onFieldsChange,
}: {
  spaceID: string;
  evaluationSetID: string;
  versionID: string;
  selectedIDs: string[];
  onSelectedIDsChange: (ids: string[]) => void;
  onFieldsChange?: (fields: ColumnEvalSetField[]) => void;
}) {
  const [page, setPage] = useState(1);
  const service = useRequest(async () => {
    const [items, version] = await Promise.all([
      StoneEvaluationApi.ListEvaluationSetItems({
        workspace_id: spaceID,
        evaluation_set_id: evaluationSetID,
        version_id: versionID,
        page_number: page,
        page_size: PAGE_SIZE,
      }),
      StoneEvaluationApi.GetEvaluationSetVersion({
        workspace_id: spaceID,
        evaluation_set_id: evaluationSetID,
        version_id: versionID,
      }),
    ]);
    return { items, fields: version.version?.evaluation_set_schema?.field_schemas ?? [] };
  }, {
    ready: Boolean(spaceID && evaluationSetID && versionID),
    refreshDeps: [spaceID, evaluationSetID, versionID, page],
  });

  useEffect(() => {
    onFieldsChange?.((service.data?.fields ?? []) as ColumnEvalSetField[]);
  }, [onFieldsChange, service.data?.fields]);

  const fields = (service.data?.fields ?? []).slice(0, 4);
  const rows = useMemo(() => service.data?.items.items ?? [], [service.data?.items.items]);
  const pageIDs = rows.map(item => item.item_id || '').filter(Boolean);
  const allPageSelected = pageIDs.length > 0 && pageIDs.every(id => selectedIDs.includes(id));

  return (
    <div className="flex min-h-[420px] flex-col gap-3">
      <Typography.Text type="secondary">
        {I18n.t('smart_optimize_selected_count', '已选')} {selectedIDs.length} {I18n.t('item_unit', '条数据')}
      </Typography.Text>
      <Spin spinning={service.loading}>
        <div className="overflow-auto rounded-lg border border-solid border-[var(--coz-stroke-primary)]">
          <table className="w-full table-fixed border-collapse text-left">
            <thead className="bg-[var(--coz-mg-secondary)]"><tr>
              <th className="w-12 p-3"><Checkbox checked={allPageSelected} onChange={event => {
                const checked = Boolean(event.target.checked);
                onSelectedIDsChange(checked ? Array.from(new Set([...selectedIDs, ...pageIDs])) : selectedIDs.filter(id => !pageIDs.includes(id)));
              }} /></th>
              <th className="w-36 p-3">ID</th>
              {fields.map(field => <th key={field.key} className="p-3">{field.name || field.key}</th>)}
            </tr></thead>
            <tbody>{rows.map(item => {
              const id = item.item_id || '';
              const values = item.turns?.[0]?.field_data_list ?? [];
              return <tr key={id} className="border-0 border-t border-solid border-[var(--coz-stroke-primary)]">
                <td className="p-3"><Checkbox checked={selectedIDs.includes(id)} onChange={event => onSelectedIDsChange(event.target.checked ? Array.from(new Set([...selectedIDs, id])) : selectedIDs.filter(selectedID => selectedID !== id))} /></td>
                <td className="truncate p-3">{id}</td>
                {fields.map(field => <td key={field.key} className="p-3"><ContentPreview content={values.find(value => value.key === field.key)?.content} /></td>)}
              </tr>;
            })}</tbody>
          </table>
        </div>
      </Spin>
      <div className="flex justify-end"><Pagination currentPage={page} pageSize={PAGE_SIZE} total={Number(service.data?.items.total || 0)} onPageChange={setPage} /></div>
    </div>
  );
}
