// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import {
  TemplateType,
  VariableType,
  type Prompt,
} from '@cozeloop/api-schema/prompt';

export interface SmartOptimizePromptValidation {
  ok: boolean;
  message?: string;
}

function getPromptTemplate(prompt?: Prompt) {
  return (
    prompt?.prompt_draft?.detail?.prompt_template ||
    prompt?.prompt_commit?.detail?.prompt_template
  );
}

/** Product rules for creating a smart-optimize task from a Prompt. */
export function validatePromptForSmartOptimize(
  prompt?: Prompt,
): SmartOptimizePromptValidation {
  const template = getPromptTemplate(prompt);
  if (!template) {
    return { ok: false, message: 'Prompt 模板为空，无法开始智能优化' };
  }
  if (template.template_type === TemplateType.Jinja2) {
    return { ok: false, message: '智能优化暂不支持 Jinja2 模板' };
  }
  const vars = template.variable_defs ?? [];
  if (!vars.length) {
    return { ok: false, message: '至少需要一个可映射的 Prompt 变量' };
  }
  if (vars.some(item => item.type === VariableType.MultiPart)) {
    return {
      ok: false,
      message: '智能优化暂不支持多模态变量（样本多模态仍可用）',
    };
  }
  if (vars.some(item => item.type && item.type !== VariableType.String)) {
    return { ok: false, message: '智能优化当前仅支持 String 类型变量' };
  }
  return { ok: true };
}
