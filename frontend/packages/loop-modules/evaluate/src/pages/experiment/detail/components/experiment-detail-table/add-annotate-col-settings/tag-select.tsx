// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { useRef, useState } from 'react';

import { debounce } from 'lodash-es';
import classNames from 'classnames';
import { useRequest } from 'ahooks';
import { I18n } from '@cozeloop/i18n-adapter';
import { tag } from '@cozeloop/api-schema/data';
import { DataApi, StoneEvaluationApi } from '@cozeloop/api-schema';
import { IconCozRefresh } from '@coze-arch/coze-design/icons';
import {
  Button,
  Select,
  Tooltip,
  Typography,
  type SelectProps,
} from '@coze-arch/coze-design';

import AnnotateItem from './annotate-item';

export interface TagSelectProps
  extends Omit<SelectProps, 'value' | 'onChange'> {
  spaceID: string;
  experimentID: string;
  tags?: tag.TagInfo[];
  onAdd?: (value: tag.TagInfo) => void;
}
export function TagSelect({
  spaceID,
  experimentID,
  tags,
  onAdd,
  ...selectProps
}: TagSelectProps) {
  const [dropdownVisible, setDropdownVisible] = useState(false);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const selectRef = useRef<any>(null);

  const addTag = useRequest(
    (tagID?: string) =>
      StoneEvaluationApi.AssociateAnnotationTag({
        workspace_id: spaceID,
        expt_id: experimentID,
        tag_key_id: tagID,
      }),
    {
      manual: true,
    },
  );

  const search = useRequest(
    async (key: string) => {
      const res = await DataApi.SearchTags({
        workspace_id: spaceID,
        page_number: 1,
        page_size: 50,
        tag_key_name_like: key,
        order_by: {
          field: 'updated_at',
        },
      });

      return (res.tagInfos || [])
        .filter(info => {
          const result = tags?.find(
            item => item.tag_key_id === info.tag_key_id,
          );
          return !result;
        })
        .map(item => ({
          label: (
            <TagSelectOption
              item={item}
              onAdd={async val => {
                await addTag.runAsync(val.tag_key_id);
                selectRef.current?.close();
                onAdd?.(val);
              }}
            />
          ),

          value: item.tag_key_id,
        }));
    },
    {
      // 需要等待 tags 获取完成，再请求
      ready: !!tags,
      refreshDeps: [tags],
    },
  );

  const handleSearch = (key: string) => {
    search.run(key);
  };

  return (
    <Select
      {...selectProps}
      ref={selectRef}
      loading={search.loading}
      filter
      remote
      suffix={
        dropdownVisible ? (
          <Tooltip theme="dark" content={I18n.t('refresh')}>
            <div className="flex flex-row items-center">
              <Button
                className="!h-6 !w-6"
                icon={<IconCozRefresh />}
                size="small"
                color="secondary"
                onClick={() => search.run('')}
              />

              <div className="h-3 w-0 border-0 border-l border-solid coz-stroke-primary ml-[2px]" />
            </div>
          </Tooltip>
        ) : null
      }
      onDropdownVisibleChange={setDropdownVisible}
      optionList={search.data}
      onSearch={debounce(handleSearch, 500)}
    />
  );
}

function TagSelectOption({
  item,
  onAdd,
}: {
  item: tag.TagInfo;
  onAdd: (item: tag.TagInfo) => Promise<void>;
}) {
  const [adding, setAdding] = useState(false);
  const disabled = item.status !== tag.TagStatus.Active;
  return (
    <div
      className="group w-full hover:bg-brand-3 rounded-[6px] max-w-[600px]"
      onClick={e => {
        e.preventDefault();
        e.stopPropagation();
      }}
    >
      <AnnotateItem
        data={item}
        actions={
          <div className="ml-6 whitespace-nowrap invisible group-hover:visible">
            <Typography.Text
              link
              className={classNames('text-[13px]', {
                '!text-brand-7': disabled,
              })}
              disabled={adding || disabled}
              onClick={async e => {
                e.stopPropagation();
                if (adding || disabled) {
                  return;
                }
                try {
                  setAdding(true);
                  await onAdd(item);
                  setAdding(false);
                } catch (error) {
                  setAdding(false);
                }
              }}
            >
              {I18n.t('space_member_role_type_add_btn')}
            </Typography.Text>
          </div>
        }
        disabled={disabled}
      />
    </div>
  );
}
