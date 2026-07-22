// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { createRsbuildConfig } from '@cozeloop/rsbuild-config';

const port = 8090;

export default createRsbuildConfig({
  server: {
    port,
    cors: {
      origin: '*',
    },
    proxy: {
      '/api': {
        target: 'http://localhost:8888',
        changeOrigin: true,
      },
      '/v1': {
        target: 'http://localhost:8888',
        changeOrigin: true,
      },
      // Signed MinIO upload/download URLs are path-style under /{bucket}/...
      // In Docker they go through nginx :8082; hot-reload :8090 must proxy them too,
      // otherwise PUT "succeeds" in the UI but the object never lands and LLM image
      // prefetch returns HTTP 404 (surfaced as 模型调用错误 / request not valid).
      '/cozeloop-minio': {
        target: 'http://localhost:8082',
        changeOrigin: true,
      },
    },
  },
  dev: {
    lazyCompilation: false,
    assetPrefix: `http://localhost:${port}`,
    client: {
      port: `${port}`,
      host: 'localhost',
      protocol: 'ws',
    },
  },
  html: {
    title: 'Prompt Loop',
    template: './src/assets/template.html',
    favicon: './src/assets/images/prompt-loop.svg',
    crossorigin: 'anonymous',
  },
});
