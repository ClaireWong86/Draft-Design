// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { TypographyText } from '@cozeloop/shared-components';
import {
  type ColumnAnnotation,
  type AnnotateRecord,
} from '@cozeloop/api-schema/evaluation';
import { Divider, Popover } from '@coze-arch/coze-design';

import { TagRender } from './tag/tag-render';

interface NameScoreTagProps {
  name?: string;
  annotation?: ColumnAnnotation;
  annotateRecord?: AnnotateRecord;
  border?: boolean;
}

export function AnnotationNameScoreTag({
  name,
  annotation,
  annotateRecord,
  border = true,
}: NameScoreTagProps) {
  const borderClass = border
    ? 'border border-solid border-[var(--coz-stroke-primary)] cursor-pointer hover:bg-[var(--coz-mg-primary)] hover:border-[var(--coz-stroke-plus)]'
    : '';
  return (
    <div className={'group flex items-center text-[var(--coz-fg-primary)]'}>
      <div
        className={`flex items-center h-5 px-2 rounded-[3px] gap-1 text-xs font-medium ${borderClass}`}
      >
        <TypographyText className="max-w-10">{name ?? '-'}</TypographyText>
        <Divider layout="vertical" style={{ height: 12 }} />
        {annotation ? (
          <TagRender
            className="!max-w-[100px] overflow-hidden"
            annotation={annotation}
            annotateRecord={annotateRecord}
          />
        ) : null}
      </div>
    </div>
  );
}

export function AnnotationNameScore({
  annotation,
  annotationResult,
  enablePopover = false,
  border = true,
  defaultShowAction: _defaultShowAction,
}: {
  annotation?: ColumnAnnotation;
  annotationResult?: AnnotateRecord;
  enablePopover?: boolean;
  border?: boolean;
  defaultShowAction?: boolean;
}) {
  if (!enablePopover) {
    return (
      <AnnotationNameScoreTag
        name={annotation?.tag_key_name}
        annotation={annotation}
        annotateRecord={annotationResult}
        border={border}
      />
    );
  }
  return (
    <Popover
      position="top"
      trigger="click"
      stopPropagation
      content={
        <div className="p-1" style={{ color: 'var(--coz-fg-secondary)' }}>
          <AnnotationNameScoreTag
            name={annotation?.tag_key_name}
            annotation={annotation}
            annotateRecord={annotationResult}
            border={false}
          />
        </div>
      }
    >
      <div>
        <AnnotationNameScoreTag
          name={annotation?.tag_key_name}
          annotation={annotation}
          annotateRecord={annotationResult}
          border={border}
        />
      </div>
    </Popover>
  );
}
