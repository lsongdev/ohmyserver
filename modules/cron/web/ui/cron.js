// Job scheduler UI helpers shared by the pages.
import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Status } from '@lsongdev/panel/ui.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

const RUN = {
  running: ['Running', 'info'], succeeded: ['Succeeded', 'success'], failed: ['Failed', 'danger'],
  timeout: ['Timed out', 'danger'], canceled: ['Canceled', undefined], interrupted: ['Interrupted', 'warning'],
};

export function RunStatus({ run }) {
  const [label, tone] = RUN[run.status] || [run.status, undefined];
  const title = run.status === 'failed' && !run.error ? `Exit code ${run.exit_code}` : run.error;
  return html`<${Status} state=${run.status} label=${label} variant=${tone} title=${title} />`;
}

export const TRIGGERS = { schedule: 'Schedule', manual: 'Manual' };

export function datetime(value) {
  if (!value) return '—';
  return new Date(value).toLocaleString('zh-CN', { hour12: false });
}

export function duration(run) {
  if (!run.finished_at) return '—';
  const ms = new Date(run.finished_at) - new Date(run.started_at);
  if (ms < 1000) return `${ms} ms`;
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s}s` : s < 3600 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
}

// relative formats a future time as "in 3 minutes".
export function relative(value) {
  if (!value) return '';
  const s = Math.round((new Date(value) - Date.now()) / 1000);
  if (s < 60) return s <= 0 ? 'now' : `in ${s} seconds`;
  if (s < 3600) return `in ${Math.round(s / 60)} minutes`;
  if (s < 86400) return `in ${Math.round(s / 3600)} hours`;
  return `in ${Math.round(s / 86400)} days`;
}
