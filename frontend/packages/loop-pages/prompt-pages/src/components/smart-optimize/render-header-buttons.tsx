// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { type ReactNode } from 'react';

import { type OptimizeSourceType } from './types';
import { SmartOptimizeHeaderDropdown } from './header-dropdown';

/**
 * Insert Smart Optimize between version history and submit.
 * Expected button order: [compare, versionList, submit]
 */
export function renderSmartOptimizeHeaderButtons(
  currentButtons: ReactNode[],
  ctx: {
    onOpenWizard: (sourceType: OptimizeSourceType) => void;
  },
): ReactNode {
  const [compare, versionList, submit, ...rest] = currentButtons;
  return (
    <>
      {compare}
      {versionList}
      <SmartOptimizeHeaderDropdown onOpenWizard={ctx.onOpenWizard} />
      {submit}
      {rest}
    </>
  );
}
