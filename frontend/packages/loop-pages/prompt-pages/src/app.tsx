// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { Routes, Route, Navigate } from 'react-router-dom';
import { lazy } from 'react';

const PromptListPage = lazy(() => import('./pages/list'));
const PromptDevelopPage = lazy(() => import('./pages/develop'));
const PromptPlaygroundPage = lazy(() => import('./pages/playground'));
const SmartOptimizeCreatePage = lazy(
  () => import('./pages/smart-optimize-create'),
);

const App = () => (
  <div className="text-sm h-full overflow-hidden">
    <Routes>
      <Route path="" element={<Navigate to="prompts" replace />} />
      {/* PE 列表 */}
      <Route path="prompts" element={<PromptListPage />} />
      <Route
        path="prompts/:promptID/optimize/create"
        element={<SmartOptimizeCreatePage />}
      />
      <Route path="prompts/:promptID" element={<PromptDevelopPage />} />
      <Route path="playground" element={<PromptPlaygroundPage />} />
    </Routes>
  </div>
);

export default App;
