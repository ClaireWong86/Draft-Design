// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { type ComponentType, type ReactNode } from 'react';

import { type Prompt } from '@cozeloop/api-schema/prompt';

export type SmartOptimizeSourceType = 'experiment' | 'eval_set';

export interface SmartOptimizeHeaderDropdownProps {
  prompt?: Prompt;
  spaceID?: string;
  onOpenWizard: (sourceType: SmartOptimizeSourceType) => void;
}

export interface SmartOptimizeTaskPanelProps {
  promptID?: string;
  spaceID?: string;
  onAdoptSuccess?: () => void;
}

export interface SmartOptimizeWizardProps {
  visible: boolean;
  sourceType: SmartOptimizeSourceType;
  prompt?: Prompt;
  spaceID?: string;
  onClose: () => void;
  onSubmitted?: (taskId: string) => void;
}

export interface PromptAdapters {
  SmartOptimizeHeaderDropdown?: ComponentType<SmartOptimizeHeaderDropdownProps>;
  SmartOptimizeTaskPanel?: ComponentType<SmartOptimizeTaskPanelProps>;
  SmartOptimizeWizard?: ComponentType<SmartOptimizeWizardProps>;
  renderSmartOptimizeHeaderButtons?: (
    currentButtons: ReactNode[],
    prompt: Prompt | undefined,
    ctx: {
      spaceID?: string;
      onOpenWizard: (sourceType: SmartOptimizeSourceType) => void;
    },
  ) => ReactNode;
}
