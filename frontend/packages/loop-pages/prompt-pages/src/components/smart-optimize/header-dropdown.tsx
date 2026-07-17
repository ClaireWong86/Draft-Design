// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { I18n } from '@cozeloop/i18n-adapter';
import { IconCozArrowDown } from '@coze-arch/coze-design/icons';
import { Button, Dropdown, Typography } from '@coze-arch/coze-design';

import { type OptimizeSourceType } from './types';

const PATHS: Array<{
  type: OptimizeSourceType;
  titleKey: string;
  titleFallback: string;
  descKey: string;
  descFallback: string;
}> = [
  {
    type: 'experiment',
    titleKey: 'smart_optimize_by_experiment',
    titleFallback: '基于评测实验优化 Prompt',
    descKey: 'smart_optimize_by_experiment_desc',
    descFallback:
      '根据批量评测的问题、模型回答、参考回答（如有）以及对应评估器得分来优化 Prompt',
  },
  {
    type: 'eval_set',
    titleKey: 'smart_optimize_by_eval_set',
    titleFallback: '基于优质的评测集优化 Prompt',
    descKey: 'smart_optimize_by_eval_set_desc',
    descFallback:
      '适合仅有批量问题、模型回答与参考回答的场景；模型将据此优化 Prompt',
  },
];

export function SmartOptimizeHeaderDropdown({
  onOpenWizard,
}: {
  onOpenWizard: (sourceType: OptimizeSourceType) => void;
}) {
  return (
    <Dropdown
      trigger="click"
      position="bottomRight"
      showTick={false}
      clickToHide
      zIndex={20}
      render={
        <Dropdown.Menu className="!w-[360px] !max-w-[90vw]">
          {PATHS.map(item => (
            <Dropdown.Item
              key={item.type}
              className="!px-3 !py-2 !h-auto !whitespace-normal"
              onClick={() => onOpenWizard(item.type)}
            >
              <div className="flex flex-col gap-1 py-1">
                <Typography.Text strong>
                  {I18n.t(item.titleKey, item.titleFallback)}
                </Typography.Text>
                <Typography.Text
                  type="secondary"
                  size="small"
                  className="!whitespace-normal"
                >
                  {I18n.t(item.descKey, item.descFallback)}
                </Typography.Text>
              </div>
            </Dropdown.Item>
          ))}
        </Dropdown.Menu>
      }
    >
      <Button color="brand" icon={<IconCozArrowDown />} iconPosition="right">
        {I18n.t('smart_optimize', '智能优化')}
      </Button>
    </Dropdown>
  );
}
