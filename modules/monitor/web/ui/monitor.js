// Host monitor UI helpers: formatting, status, meters and charts.
import { html, useState } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Status } from '@lsongdev/panel/ui.js';
import { bytes } from '@lsongdev/panel/format.js';

// useApi loads a GET endpoint, polling every interval milliseconds.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => api.get(path, { signal }), [path], { interval });
}

export { bytes };

export const rate = v => (v == null ? '—' : `${bytes(v)}/s`);

export const pct = (used, total) => (total ? (used / total) * 100 : 0);

export const fixed = v => (v == null ? '—' : v < 10 ? v.toFixed(1) : Math.round(v).toString());

export function uptime(seconds) {
  if (!seconds) return '—';
  const d = Math.floor(seconds / 86400), h = Math.floor((seconds % 86400) / 3600), m = Math.floor((seconds % 3600) / 60);
  if (d) return `${d}d ${h}h`;
  if (h) return `${h}h ${m}m`;
  return `${m}m`;
}

const STATUS = {
  online: ['Online', 'green'], offline: ['Offline', 'red'], pending: ['Collecting', 'blue'],
  disabled: ['Disabled', 'gray'], gone: ['Removed', 'gray'],
};

export function HostStatus({ host }) {
  const [label, tone] = STATUS[host.status] || [host.status, 'gray'];
  return html`<${Status} state=${host.status} label=${label} tone=${tone} title=${host.error} />`;
}

export const KINDS = { local: 'Local', ssh: 'SSH' };

// Meter is a horizontal bar, amber from 80% and red from 90%.
export function Meter({ value }) {
  const tone = value >= 90 ? 'danger' : value >= 80 ? 'warn' : '';
  return html`<div class="meter"><span class=${tone} style=${`width:${Math.min(100, Math.max(0, value))}%`}></span></div>`;
}

// Sparkline draws values of 0-100 as a small area chart.
export function Sparkline({ values, color = 'var(--series-1)' }) {
  if (!values?.length) return html`<svg class="spark"></svg>`;
  const n = Math.max(values.length - 1, 1);
  const pts = values.map((v, i) => `${(i / n) * 100},${36 - (Math.min(v, 100) / 100) * 34}`).join(' ');
  return html`<svg class="spark" viewBox="0 0 100 36" preserveAspectRatio="none">
    <polygon points=${`0,36 ${pts} 100,36`} fill=${color} />
    <polyline points=${pts} stroke=${color} />
  </svg>`;
}

function clock(t, long) {
  return new Date(t).toLocaleString('zh-CN', long
    ? { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }
    : { hour: '2-digit', minute: '2-digit', hour12: false });
}

// Chart draws series over time. points: [{t, ...}]; series: [{label,
// value(point), color}]; format formats a value; max fixes the top of the
// scale (e.g. 100 for percentages). Points further apart than two steps
// leave a gap, so missing data is not drawn as a straight line.
export function Chart({ title, points, series, format, max, step, from, to }) {
  const [hover, setHover] = useState(null);
  const W = 1000, H = 160;
  const t0 = from ?? (points.length ? new Date(points[0].t).getTime() : 0);
  const t1 = to ?? (points.length ? new Date(points[points.length - 1].t).getTime() : 1);
  const span = Math.max(t1 - t0, 1);
  const values = points.flatMap(p => series.map(s => s.value(p) ?? 0));
  const top = max ?? niceMax(Math.max(0, ...values));
  const x = t => ((t - t0) / span) * W;
  const y = v => H - (Math.min(v, top) / top) * H;

  // Split into runs without gaps.
  const runs = [];
  let run = [];
  points.forEach((p, i) => {
    const t = new Date(p.t).getTime();
    if (i > 0 && t - new Date(points[i - 1].t).getTime() > 2.5 * step * 1000) {
      runs.push(run);
      run = [];
    }
    run.push(p);
  });
  if (run.length) runs.push(run);

  const shown = hover ?? points[points.length - 1];
  const onMove = e => {
    if (!points.length) return;
    const box = e.currentTarget.getBoundingClientRect();
    const t = t0 + ((e.clientX - box.left) / box.width) * span;
    let best = points[0];
    for (const p of points) if (Math.abs(new Date(p.t) - t) < Math.abs(new Date(best.t) - t)) best = p;
    setHover(best);
  };
  return html`<div class="card">
    <div class="chart-head">
      <h2>${title}</h2>
      <div class="legend">
        ${shown ? html`<span>${clock(shown.t, span > 86400000)}</span>` : null}
        ${series.map(s => html`<span key=${s.label}><i style=${`background:${s.color}`}></i>${s.label}<b>${shown ? format(s.value(shown)) : '—'}</b></span>`)}
      </div>
    </div>
    ${!points.length ? html`<div class="chart-empty">No data yet</div>` : html`
      <div class="plot" onMouseMove=${onMove} onMouseLeave=${() => setHover(null)}>
        <svg viewBox=${`0 0 ${W} ${H}`} preserveAspectRatio="none">
          <g class="grid">${[0, 0.5, 1].map(f => html`<line key=${f} x1="0" x2=${W} y1=${H * f} y2=${H * f} />`)}</g>
          ${series.map(s => runs.map((r, i) => {
            const pts = r.map(p => `${x(new Date(p.t).getTime())},${y(s.value(p) ?? 0)}`).join(' ');
            const first = x(new Date(r[0].t).getTime()), last = x(new Date(r[r.length - 1].t).getTime());
            return html`<g key=${s.label + i}>
              ${series.length === 1 ? html`<polygon class="area" fill=${s.color} points=${`${first},${H} ${pts} ${last},${H}`} />` : null}
              <polyline class="line" stroke=${s.color} points=${pts} />
            </g>`;
          }))}
          ${hover ? html`<line class="cursor" x1=${x(new Date(hover.t).getTime())} x2=${x(new Date(hover.t).getTime())} y1="0" y2=${H} />` : null}
        </svg>
        <span class="ymax">${format(top)}</span>
      </div>
      <div class="axis"><span>${clock(t0, span > 86400000)}</span><span>${clock(t0 + span / 2, span > 86400000)}</span><span>${clock(t1, span > 86400000)}</span></div>`}
  </div>`;
}

// niceMax rounds a scale's top up to 1, 2 or 5 times a power of ten.
function niceMax(v) {
  if (v <= 0) return 1;
  const p = 10 ** Math.floor(Math.log10(v));
  for (const m of [1, 2, 5, 10]) if (v <= m * p) return m * p;
  return 10 * p;
}
