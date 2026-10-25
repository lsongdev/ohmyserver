// CI UI helpers shared by the pages.
import { html } from './lib/html.js';
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Status } from './lib/ui.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => api.get(path, { signal }), [path], { interval });
}

const STATUS = {
  pending: ['排队中', 'blue'], running: ['运行中', 'blue'], success: ['成功', 'green'], failure: ['失败', 'red'],
  error: ['错误', 'red'], canceled: ['已取消', 'gray'], skipped: ['已跳过', 'gray'],
};

export function BuildStatus({ status, title }) {
  const [label, tone] = STATUS[status] || [status, 'gray'];
  return html`<${Status} state=${status} label=${label} tone=${tone} title=${title} />`;
}

export const active = status => status === 'pending' || status === 'running';

export const EVENTS = { push: '推送', manual: '手动' };

export function duration(from, to) {
  if (!from) return '—';
  const s = Math.max(0, Math.round(((to ? new Date(to) : new Date()) - new Date(from)) / 1000));
  return s < 60 ? `${s}s` : s < 3600 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
}

export function datetime(value) {
  return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—';
}

export const short = sha => (sha || '').slice(0, 8);
