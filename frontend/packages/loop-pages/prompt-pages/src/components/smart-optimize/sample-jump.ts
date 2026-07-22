// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable @typescript-eslint/no-magic-numbers -- case_id split item_id_turn_id */
import { type OptimizeSource } from './types';

export function parseCaseIdForJump(caseId?: string): {
  itemId: string;
  turnId?: string;
} | null {
  if (!caseId) {
    return null;
  }
  const parts = caseId.split('_');
  if (parts.length >= 2 && /^\d+$/.test(parts[0] || '')) {
    return { itemId: parts[0], turnId: parts.slice(1).join('_') };
  }
  return { itemId: caseId };
}

/** Module-relative path for opening the original experiment / eval-set sample. */
export function buildSampleJumpPath(
  source: OptimizeSource | undefined,
  caseId: string | undefined,
): string | undefined {
  const parsed = parseCaseIdForJump(caseId);
  if (!parsed || !source) {
    return undefined;
  }
  const { itemId } = parsed;
  if (source.type === 'experiment' && source.experiment_id) {
    return `evaluation/experiments/${source.experiment_id}?tabKey=detail&item_id=${encodeURIComponent(itemId)}`;
  }
  if (source.type === 'eval_set' && source.eval_set_id) {
    const version = source.eval_set_version_id
      ? `&version=${encodeURIComponent(source.eval_set_version_id)}`
      : '';
    return `evaluation/datasets/${source.eval_set_id}?item_id=${encodeURIComponent(itemId)}${version}`;
  }
  return undefined;
}
