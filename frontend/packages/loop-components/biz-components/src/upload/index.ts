// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { nanoid } from 'nanoid';
import { BusinessType } from '@cozeloop/api-schema/foundation';
import { FoundationApi } from '@cozeloop/api-schema';
import { type customRequestArgs } from '@coze-arch/coze-design';

/**
 * The backend embeds the object key into the presigned URL path WITHOUT
 * percent-encoding it. Spaces / non-ASCII (e.g. macOS "截屏 2026-xx.png") make
 * the URL unusable, and "#" truncates the path so the SigV4 signature no longer
 * matches (MinIO returns 403 AccessDenied). Keep only URL-safe chars in the
 * filename; nanoid already guarantees the key is unique.
 */
function sanitizeFileName(name: string): string {
  const lastDot = name.lastIndexOf('.');
  const hasExt = lastDot > 0 && lastDot < name.length - 1;
  const base = hasExt ? name.slice(0, lastDot) : name;
  const ext = hasExt ? name.slice(lastDot + 1) : '';
  const safe = (s: string) => s.replace(/[^A-Za-z0-9._-]/g, '_');
  const safeBase = safe(base).replace(/^_+|_+$/g, '') || 'file';
  const safeExt = safe(ext);
  return safeExt ? `${safeBase}.${safeExt}` : safeBase;
}

export function uploadFile({
  file,
  fileType = 'image',
  onProgress,
  onSuccess,
  onError,
  spaceID,
  businessType = BusinessType.Evaluation,
}: {
  file: File;
  fileType?: 'image' | 'object';
  onProgress?: customRequestArgs['onProgress'];
  onSuccess?: customRequestArgs['onSuccess'];
  onError?: customRequestArgs['onError'];
  spaceID: string;
  businessType?: BusinessType;
}) {
  const result = new Promise<string>((resolve, reject) => {
    (async function () {
      try {
        const key = `${spaceID}/${nanoid()}/${sanitizeFileName(file.name)}`;
        const res = await FoundationApi.SignUploadFile({
          keys: [key],
          business_type: businessType,
          workspace_id: spaceID,
        });
        const fileUrl = res?.uris?.[0];
        if (!fileUrl) {
          throw new Error('fileUrl is empty');
        }
        const uploadResp = await fetch(fileUrl, {
          method: 'PUT',
          body: file,
        });
        if (!uploadResp.ok) {
          throw new Error(
            `file upload failed: HTTP ${uploadResp.status} ${uploadResp.statusText}`,
          );
        }
        onSuccess?.({ status: 200, Uri: key });
        resolve(key ?? '');
      } catch (e) {
        onError?.({ status: 500 });
        reject(e);
      }
    })();
  });
  return result;
}
