// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import {
  type ListUserSpaceRequest,
  SpaceType,
} from '@cozeloop/api-schema/foundation';
import { FoundationApi } from '@cozeloop/api-schema';

const LOCAL_DEV_SPACE = {
  id: 'local-dev-space',
  name: '本地开发空间',
  description: 'Local development space',
  space_type: SpaceType.Personal,
  owner_user_id: 'local-dev-user',
  enterprise_id: 'personal',
};

export const spaceService = (() => ({
  async getSpace(spaceId: number, appId?: number) {
    const resp = await FoundationApi.GetSpace({
      space_id: spaceId,
    }).catch(() => ({ space: LOCAL_DEV_SPACE }));

    return resp.space;
  },
  listSpaces(req?: ListUserSpaceRequest) {
    return FoundationApi.ListUserSpaces({
      page_number: 1,
      page_size: 100,
      ...req,
    }).catch(() => ({
      spaces: [LOCAL_DEV_SPACE],
      total: 1,
    }));
  },
}))();
