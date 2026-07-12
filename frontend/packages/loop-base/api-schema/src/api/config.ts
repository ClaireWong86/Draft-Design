// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { createAPI as apiFactory } from '@coze-arch/idl2ts-runtime';
import { type IMeta } from '@coze-arch/idl2ts-runtime';

import {
  checkResponseData,
  checkFetchResponse,
  onClientError,
  onClientBizError,
} from '../notification';

export interface ApiOption {
  /**
   * error toast config
   * @default false
   */
  disableErrorToast?: boolean;
  /** headers */
  headers?: Record<string, string>;
}

export interface ApiResponse {
  code?: number;
  msg?: string;
}

function getBaseUrl() {
  try {
    return process.env.API_SCHEMA_BASE_URL || '';
    // eslint-disable-next-line @coze-arch/use-error-in-catch -- no-catch
  } catch {
    return '';
  }
}

function shouldUseLocalDevFallback(uri: string, error: unknown) {
  if (getBaseUrl()) {
    return false;
  }

  const isLocalHost =
    globalThis.location?.hostname === 'localhost' ||
    globalThis.location?.hostname === '127.0.0.1';
  const isUnavailableApi =
    error instanceof SyntaxError ||
    (error instanceof Error && error.message === 'NotFound');

  return isLocalHost && uri.startsWith('/api/') && isUnavailableApi;
}

function getLocalDevFallback() {
  return {
    code: 0,
    total: 0,
    has_more: false,
    next_page_token: '',
    user_info: {
      user_id: 'local-dev-user',
      name: 'local-dev-user',
      nick_name: '本地开发用户',
      email: 'local-dev@example.com',
    },
    spaces: [
      {
        id: 'local-dev-space',
        name: '本地开发空间',
        description: 'Local development space',
        space_type: 1,
        owner_user_id: 'local-dev-user',
        enterprise_id: 'personal',
      },
    ],
    models: [],
    users: [],
    prompts: [],
    datasets: [],
    evaluators: [],
    experiments: [],
    tagInfos: [],
    spans: [],
    traces: [],
    views: [],
    annotations: [],
    templates: [],
  };
}

export function createAPI<
  T extends {},
  K,
  O = ApiOption,
  B extends boolean = false,
>(meta: IMeta, cancelable?: B) {
  return apiFactory<T, K & ApiResponse, O, B>(meta, cancelable, false, {
    config: {
      clientFactory: _meta => async (url, init, options) => {
        const headers = {
          'Agw-Js-Conv': 'str', // RESERVED HEADER FOR SERVER
          ...init.headers,
          ...(options?.headers ?? {}),
        };
        const uri = `${getBaseUrl()}${url}`;
        const opts = { ...init, headers };

        try {
          if (init?.body) {
            opts.body = JSON.stringify(init?.body);
          }
          const resp = await fetch(uri, opts);
          checkFetchResponse(resp);

          const data = await resp.json();
          checkResponseData(uri, data);

          return data;
        } catch (e) {
          if (shouldUseLocalDevFallback(uri, e)) {
            return getLocalDevFallback();
          }

          options.disableErrorToast || onClientError(uri, e);
          onClientBizError(uri, e);
          throw e;
        }
      },
    },
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- skip
  } as any);
}
