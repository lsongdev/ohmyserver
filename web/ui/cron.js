// Job scheduler UI helpers shared by the pages.
import { html } from './lib/html.js';
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Status } from './lib/ui.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

const RUN = {
  running: ['运行中', 'blue'], succeeded: ['成功', 'green'], failed: ['失败', 'red'],
  timeout: ['超时', 'red'], canceled: ['已取消', 'gray'], interrupted: ['已中断', 'amber'],
};

export function RunStatus({ run }) {
  const [label, tone] = RUN[run.status] || [run.status, 'gray'];
  const title = run.status === 'failed' && !run.error ? `退出码 ${run.exit_code}` : run.error;
  return html`<${Status} state=${run.status} label=${label} tone=${tone} title=${title} />`;
}

export const TRIGGERS = { schedule: '定时', manual: '手动' };

export function datetime(value) {
  if (!value) return '—';
  return new Date(value).toLocaleString('zh-CN', { hour12: false });
}

export function duration(run) {
  if (!run.finished_at) return '—';
  const ms = new Date(run.finished_at) - new Date(run.started_at);
  if (ms < 1000) return `${ms} ms`;
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s} 秒` : s < 3600 ? `${Math.floor(s / 60)} 分 ${s % 60} 秒` : `${Math.floor(s / 3600)} 小时 ${Math.floor((s % 3600) / 60)} 分`;
}

// relative formats a future time as "3 分钟后".
export function relative(value) {
  if (!value) return '';
  const s = Math.round((new Date(value) - Date.now()) / 1000);
  if (s < 60) return s <= 0 ? '即将' : `${s} 秒后`;
  if (s < 3600) return `${Math.round(s / 60)} 分钟后`;
  if (s < 86400) return `${Math.round(s / 3600)} 小时后`;
  return `${Math.round(s / 86400)} 天后`;
}
