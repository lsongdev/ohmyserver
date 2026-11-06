// Service manager UI helpers shared by the pages.
import { html, useEffect, useRef, useState } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Alert } from '@lsongdev/scripts/react/components/index.js';
import { Status } from '@lsongdev/panel/ui.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

// unitLabel drops the ".service" every name ends with.
export const unitLabel = name => name.replace(/\.service$/, '');

// ServiceStatus shows a service's state as systemctl reports it.
export function ServiceStatus({ unit }) {
  const { active, sub } = unit;
  if (active === 'active' && sub === 'running') return html`<${Status} state="running" label="Running" />`;
  if (active === 'active') return html`<${Status} state="active" label=${`Active (${sub})`} />`;
  if (active === 'failed') return html`<${Status} state="failed" label="Failed" />`;
  if (active === 'activating' || active === 'deactivating' || active === 'reloading') return html`<${Status} state="starting" label=${active[0].toUpperCase() + active.slice(1)} />`;
  return html`<${Status} state="stopped" label="Stopped" />`;
}

export const isActive = unit => ['active', 'activating', 'reloading'].includes(unit.active);

export function Unsupported() {
  return html`<${Alert} variant="info">systemd is not running on this system, so there are no services to manage.<//>`;
}

const PRIORITY = ['emerg', 'alert', 'crit', 'error', 'warning', 'notice', 'info', 'debug'];
const tone = p => (p <= 3 ? 'p-error' : p === 4 ? 'p-warning' : p === 7 ? 'p-debug' : '');
const pad = n => String(n).padStart(2, '0');
const stamp = t => { const d = new Date(t); return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`; };

// Keep the newest entries; a live view would grow without bound.
const KEEP = 2000;

// Journal shows the entries a query selects ({ unit, priority, since, q,
// lines }) and, when live, follows new ones. It scrolls with new entries
// while the reader is at the bottom.
export function Journal({ query, live, showUnit = true }) {
  const { api, client } = usePanel();
  const [entries, setEntries] = useState(null);
  const [error, setError] = useState(null);
  const [state, setState] = useState('loading'); // loading, live, paused, reconnecting
  const box = useRef(null);
  const atBottom = useRef(true);
  const key = JSON.stringify(query);

  useEffect(() => {
    let closed = false, source = null, timer = null, last = '';
    const controller = new AbortController();
    setEntries(null);
    setError(null);
    setState('loading');
    const add = list => setEntries(prev => {
      const next = (prev || []).concat(list);
      return next.length > KEEP ? next.slice(next.length - KEEP) : next;
    });
    const follow = () => {
      if (closed) return;
      source = new EventSource(client.url('logs/stream', { ...query, lines: undefined, after: last }));
      source.addEventListener('open', () => setState('live'));
      source.addEventListener('entry', e => {
        const entry = JSON.parse(e.data);
        last = entry.cursor;
        add([entry]);
      });
      source.addEventListener('error', e => {
        source.close();
        if (e.data) { setError(JSON.parse(e.data)); setState('paused'); return; }
        // Reconnect from the last entry seen, so none repeats or goes missing.
        setState('reconnecting');
        timer = setTimeout(follow, 3000);
      });
    };
    api.get('logs', { query, signal: controller.signal }).then(list => {
      if (closed) return;
      last = list.length ? list[list.length - 1].cursor : '';
      add(list);
      if (live) follow(); else setState('paused');
    }).catch(err => { if (err.name !== 'AbortError') { setError(err); setState('paused'); } });
    return () => { closed = true; controller.abort(); source?.close(); clearTimeout(timer); };
  }, [key, live]);

  useEffect(() => {
    const el = box.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [entries]);

  const onScroll = e => {
    const el = e.currentTarget;
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
  };
  return html`<div>
    <${Alert} error=${error} />
    <div class="log journal" ref=${box} onScroll=${onScroll}>
      ${entries === null ? html`<div class="journal-empty">Loading…</div>`
        : entries.length === 0 ? html`<div class="journal-empty">No entries${live ? ' yet. New ones appear here.' : '.'}</div>`
        : entries.map(e => html`<div class="journal-line ${tone(e.priority)}" key=${e.cursor} title=${PRIORITY[e.priority]}>
            <span class="journal-time">${stamp(e.time)}</span>${' '}
            ${showUnit ? html`<span class="journal-ident">${e.identifier || unitLabel(e.unit || '')}${e.pid ? `[${e.pid}]` : ''}:</span>${' '}` : null}${e.message}
          </div>`)}
    </div>
    <div class="journal-status muted small">
      ${state === 'live' ? html`<${Status} state="running" label="Live" />` : state === 'reconnecting' ? html`<${Status} state="starting" label="Reconnecting…" />` : null}
      ${entries ? html`<span>${entries.length} ${entries.length === 1 ? 'entry' : 'entries'}${entries.length >= KEEP ? ` (the newest ${KEEP})` : ''}</span>` : null}
    </div>
  </div>`;
}
