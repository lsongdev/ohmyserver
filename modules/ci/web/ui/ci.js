// CI UI helpers shared by the pages.
import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Status } from '@lsongdev/panel/ui.js';
export { datetime } from '@lsongdev/panel/format.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => api.get(path, { signal }), [path], { interval });
}

const STATUS = {
  pending: ['Queued', 'blue'], running: ['Running', 'blue'], success: ['Passed', 'green'], failure: ['Failed', 'red'],
  error: ['Error', 'red'], canceled: ['Canceled', 'gray'], skipped: ['Skipped', 'gray'],
};

export function BuildStatus({ status, title }) {
  const [label, tone] = STATUS[status] || [status, 'gray'];
  return html`<${Status} state=${status} label=${label} tone=${tone} title=${title} />`;
}

export const active = status => status === 'pending' || status === 'running';

export const EVENTS = { push: 'Push', manual: 'Manual' };

export function duration(from, to) {
  if (!from) return '—';
  const s = Math.max(0, Math.round(((to ? new Date(to) : new Date()) - new Date(from)) / 1000));
  return s < 60 ? `${s}s` : s < 3600 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
}

export const short = sha => (sha || '').slice(0, 8);
