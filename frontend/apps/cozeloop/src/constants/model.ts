// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/** Preferred model order for Playground. First healthy match wins. */
export const PREFERRED_MODEL_NAMES = [
  'deepseek-v4-pro',
  'Gemini-3.1-Flash-Lite',
  'gpt-5.6-luna',
  'doubao-seed-2-0-pro-260215',
];

export function sortModelsByPreference<T extends { name?: string }>(
  models: T[],
  preferredNames: string[] = PREFERRED_MODEL_NAMES,
): T[] {
  const rank = new Map(preferredNames.map((name, index) => [name, index]));
  return [...models].sort((left, right) => {
    const leftRank = rank.get(left.name ?? '') ?? Number.MAX_SAFE_INTEGER;
    const rightRank = rank.get(right.name ?? '') ?? Number.MAX_SAFE_INTEGER;
    return leftRank - rightRank;
  });
}

export function pickPreferredModel<T extends { name?: string }>(
  models: T[],
  preferredNames: string[] = PREFERRED_MODEL_NAMES,
): T | undefined {
  const sorted = sortModelsByPreference(models, preferredNames);
  return sorted[0];
}
