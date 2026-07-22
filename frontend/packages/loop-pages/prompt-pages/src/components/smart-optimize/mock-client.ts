// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0
/* eslint-disable @typescript-eslint/no-magic-numbers -- mock scores/timings */
/* eslint-disable security/detect-object-injection -- indexed localStorage tasks */
import {
  type CreateOptimizeTaskRequest,
  type OptimizePromptSnapshot,
  type OptimizeTask,
  type OptimizeTaskClient,
} from './types';

const STORAGE_KEY = 'prompt_loop_optimize_tasks_v2';
const RADIX = 36;
const ID_SLICE_START = 2;
const ID_SLICE_END = 8;
const PROGRESS_STEPS = [15, 35, 55, 75, 100] as const;
const PROGRESS_INTERVAL_MS = 600;
const DONE_PERCENT = 100;

function createId() {
  return `opt_${Date.now().toString(RADIX)}_${Math.random()
    .toString(RADIX)
    .slice(ID_SLICE_START, ID_SLICE_END)}`;
}

function loadTasks(): OptimizeTask[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }
    return JSON.parse(raw) as OptimizeTask[];
  } catch (error) {
    console.warn('load optimize tasks failed', error);
    return [];
  }
}

function saveTasks(tasks: OptimizeTask[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(tasks));
}

function sleep(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function appendOptimizationNotes(
  snapshot: OptimizePromptSnapshot,
): OptimizePromptSnapshot {
  const messages = snapshot.messages.map(message => ({
    ...message,
    parts: message.parts?.map(part => ({ ...part })),
  }));
  const targetIndex = Math.max(
    0,
    messages.findIndex(message => message.role === 'system'),
  );
  const target = messages[targetIndex] ?? { role: 'system' as const };
  const content =
    target.content?.trim() || '## Task Goal\nDetect tire defects.';
  messages[targetIndex] = {
    ...target,
    content: `${content}

## Optimization Notes (Mock)
- Emphasize evidence grounding for each defect.
- Require JSON fields: defects, position, evidence, severity, confidence.
- Prefer conservative severity when confidence < 0.6.`,
  };

  return { ...snapshot, messages };
}

function buildMockResult(
  baselinePrompt: OptimizePromptSnapshot,
): OptimizeTask['result'] {
  const after = appendOptimizationNotes(baselinePrompt);

  return {
    before_prompt: baselinePrompt,
    after_prompt: after,
    before_score: 0.62,
    after_score: 0.81,
    score_distribution: {
      before: [0.4, 0.5, 0.6, 0.7, 0.8],
      after: [0.6, 0.7, 0.8, 0.85, 0.9],
    },
    case_details: [
      {
        case_id: 'case-1',
        before_score: 0.45,
        after_score: 0.78,
        before_actual: '{"defects":[]}',
        after_actual:
          '{"defects":[{"type":"crack","position":"sidewall","severity":"medium","confidence":0.82}]}',
        reference: '{"defects":[{"type":"crack"}]}',
      },
      {
        case_id: 'case-2',
        before_score: 0.7,
        after_score: 0.84,
        before_actual: '{"defects":[{"type":"wear"}]}',
        after_actual:
          '{"defects":[{"type":"wear","position":"tread","severity":"low","confidence":0.75}]}',
        reference: '{"defects":[{"type":"wear","position":"tread"}]}',
      },
    ],
    diagnosis: {
      failure_modes: [
        'Missing position / evidence fields on low-score cases',
        'Severity overstated when confidence is low',
      ],
      suggested_instruction_changes: [
        'Add explicit JSON schema constraints',
        'Add confidence-gated severity rule',
      ],
    },
  };
}

async function simulateProgress(taskId: string) {
  for (const progress of PROGRESS_STEPS) {
    await sleep(PROGRESS_INTERVAL_MS);
    const tasks = loadTasks();
    const idx = tasks.findIndex(t => t.id === taskId);
    if (idx < 0) {
      return;
    }
    const current = tasks[idx];
    if (current.status === 'cancelled') {
      return;
    }
    const baseline = current.result?.before_prompt;
    const next: OptimizeTask = {
      ...current,
      progress,
      status: progress >= DONE_PERCENT ? 'succeeded' : 'running',
      updated_at: Date.now(),
      result:
        progress >= DONE_PERCENT && baseline
          ? buildMockResult(baseline)
          : current.result,
    };
    tasks[idx] = next;
    saveTasks(tasks);
  }
}

export const mockOptimizeTaskClient: OptimizeTaskClient = {
  createTask(req: CreateOptimizeTaskRequest) {
    const now = Date.now();
    const task: OptimizeTask = {
      id: createId(),
      name: req.name || `智能优化任务-${new Date(now).toLocaleString()}`,
      workspace_id: req.workspace_id,
      prompt_id: req.prompt_id,
      prompt_version: req.prompt_version,
      source: req.source,
      case_item_ids: req.case_item_ids,
      mapping: req.mapping,
      mode_score: req.mode_score,
      optimizer_model_id: req.optimizer_model_id,
      status: 'queued',
      progress: 0,
      created_at: now,
      updated_at: now,
      result: {
        before_prompt: req.baseline_prompt,
        after_prompt: { ...req.baseline_prompt, messages: [] },
        before_score: 0,
        after_score: 0,
        score_distribution: { before: [], after: [] },
        case_details: [],
      },
    };
    const tasks = loadTasks();
    tasks.unshift(task);
    saveTasks(tasks);
    void simulateProgress(task.id);
    return Promise.resolve(task);
  },

  listTasks(params: { workspace_id: string; prompt_id: string }) {
    const workspaceId = params.workspace_id;
    const promptId = params.prompt_id;
    return Promise.resolve(
      loadTasks().filter(
        t => t.workspace_id === workspaceId && t.prompt_id === promptId,
      ),
    );
  },

  getTask(taskId: string) {
    return Promise.resolve(loadTasks().find(t => t.id === taskId));
  },

  cancelTask(taskId: string, _workspaceId: string) {
    const tasks = loadTasks();
    const idx = tasks.findIndex(t => t.id === taskId);
    if (idx < 0) {
      return Promise.resolve();
    }
    const current = tasks[idx];
    if (current.status === 'queued' || current.status === 'running') {
      tasks[idx] = {
        ...current,
        status: 'cancelled',
        updated_at: Date.now(),
      };
      saveTasks(tasks);
    }
    return Promise.resolve();
  },

  async adoptTask(taskId: string, _workspaceId: string) {
    const task = await this.getTask(taskId);
    if (!task?.result?.after_prompt) {
      throw new Error('任务尚无可用优化结果');
    }
    return { after_prompt: task.result.after_prompt };
  },
};
