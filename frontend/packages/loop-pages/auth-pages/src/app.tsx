// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
import { Routes, Route, Navigate } from 'react-router-dom';

import { LoginPage } from './pages';
import { AuthFrame } from './components';

export function App() {
  return (
    <AuthFrame>
      <Routes>
        <Route path="login" element={<LoginPage />} />
        <Route path="*" element={<Navigate to="login" replace={true} />} />
      </Routes>
    </AuthFrame>
  );
}
