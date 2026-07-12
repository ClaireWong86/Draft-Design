// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { CozeLoopStorage } from '@cozeloop/toolkit';

export { LOGIN_PATH, CONSOLE_PATH, COZELOOP_GITHUB_URL } from './home';
export {
  PREFERRED_MODEL_NAMES,
  sortModelsByPreference,
  pickPreferredModel,
} from './model';

export const storage = new CozeLoopStorage({
  field: 'base',
});
