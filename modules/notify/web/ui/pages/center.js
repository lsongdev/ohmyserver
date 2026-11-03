import { html, useCallback, useEffect, useState } from '../lib/html.js';
import { useAction, useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Badge, Modal, Field, toast, toastError } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { ago, datetime } from '../lib/format.js';

const LEVELS = ['critical', 'warning', 'info', 'debug'];
const TONES = { critical: 'red', warning: 'amber', info: 'blue' };
const REFRESH = 15000;
const PAGE = 100;

const Level = ({ level }) => html`<${Badge} tone=${TONES[level]}>${level[0].toUpperCase() + level.slice(1)}<//>`;
const When = ({ at }) => html`<span class="nowrap" title=${datetime(at)}>${ago(at)}</span>`;

// target turns an event's link, a path within its source module, into a URL.
// It is null when that module is not mounted here.
function useTarget() {
  const { link } = usePanel();
  return (source, path) => {
    if (!path) return null;
    const [route, search = ''] = path.split('?');
    return link?.(source, route, Object.fromEntries(new URLSearchParams(search))) || null;
  };
}

const Linked = ({ href, children }) => (href ? html`<a class="notify-title" href=${href}>${children}</a>` : html`<span class="notify-title">${children}</span>`);

function Issues() {
  const { api } = usePanel();
  const target = useTarget();
  const issues = useResource(signal => api.get('/issues', { signal }), [], { interval: REFRESH });
  const [closing, setClosing] = useState(null);
  const [muting, setMuting] = useState(null);
  const muted = i => i.muted_until && new Date(i.muted_until) > new Date();
  const unmute = async issue => {
    setClosing(issue.id);
    try {
      await api.post(`/issues/${issue.id}/mute`, { minutes: 0 });
      toast('No longer muted', 'success');
      issues.reload();
    } catch (err) {
      toastError(err);
    } finally {
      setClosing(null);
    }
  };
  const close = async issue => {
    setClosing(issue.id);
    try {
      await api.post(`/issues/${issue.id}/resolve`);
      toast('Closed. If the problem is still there, it will come back.', 'success');
      issues.reload();
    } catch (err) {
      toastError(err);
    } finally {
      setClosing(null);
    }
  };
  return html`<section>
    <h2 class="section-title">Open issues${issues.data?.length ? ` (${issues.data.length})` : ''}</h2>
    <${Notice} error=${issues.error} />
    ${!issues.data ? (issues.error ? null : html`<${Loading} />`) : issues.data.length === 0 ? html`<${Empty}>Nothing needs attention.<//>` : html`
      <${Table}>
        <thead><tr><th>Level</th><th>Issue</th><th>Source</th><th>Since</th><th class="actions"></th></tr></thead>
        <tbody>${issues.data.map(i => html`<tr key=${i.id}>
          <td><${Level} level=${i.level} /></td>
          <td>
            <${Linked} href=${target(i.source, i.link)}>${i.title}<//>
            ${muted(i) ? html` <${Badge} title=${`Not sent anywhere until ${datetime(i.muted_until)}`}>Muted<//>` : null}
            ${i.body ? html`<div class="cell-sub notify-body">${i.body}</div>` : null}
          </td>
          <td>${i.source}</td>
          <td><${When} at=${i.first_seen} /></td>
          <td class="actions"><span class="notify-issue-actions">
            ${muted(i)
              ? html`<button class="button button-small" disabled=${closing === i.id} onClick=${() => unmute(i)}>Unmute</button>`
              : html`<button class="button button-small" disabled=${closing === i.id} title="Keep it from being sent anywhere for a while" onClick=${() => setMuting(i)}>Mute</button>`}
            <button class="button button-small" disabled=${closing === i.id}
              title="For a problem whose source is gone. If it is still there, it comes back."
              onClick=${() => close(i)}>Close</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
    ${muting ? html`<${MuteDialog} issue=${muting} onClose=${() => setMuting(null)} onDone=${issues.reload} />` : null}
  </section>`;
}

function MuteDialog({ issue, onClose, onDone }) {
  const { api } = usePanel();
  const [minutes, setMinutes] = useState(60);
  const { busy, error, run } = useAction();
  const close = () => {
    if (busy) return false;
    onClose();
  };
  const submit = e => {
    e.preventDefault();
    run(async () => {
      await api.post(`/issues/${issue.id}/mute`, { minutes: Number(minutes) });
      toast('Muted', 'success');
      onDone();
      onClose();
    });
  };
  return html`<${Modal} title="Mute this issue" onClose=${close} footer=${html`
    <button class="button" type="button" disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-primary" type="submit" form="mute-form" disabled=${busy}>${busy ? 'Muting…' : 'Mute'}</button>`}>
    <form id="mute-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <p style="margin:0">${issue.title}. It stays listed here, but is not sent to any channel, including if it gets worse.</p>
      <${Field} label="For">
        <select class="input" autofocus disabled=${busy} value=${minutes} onChange=${e => setMinutes(e.currentTarget.value)}>
          <option value="60">1 hour</option>
          <option value="480">8 hours</option>
          <option value="1440">1 day</option>
          <option value="10080">1 week</option>
          <option value="43200">30 days</option>
        </select>
      <//>
    </form>
  <//>`;
}

function Events() {
  const { api, query, setQuery } = usePanel();
  const target = useTarget();
  const level = query.level || '';
  const source = query.source || '';
  const search = query.q || '';
  const filter = { level, source, q: search, limit: PAGE };
  const key = JSON.stringify(filter);
  const sources = useResource(signal => api.get('/sources', { signal }), []);
  const latest = useResource(signal => api.get('/events', { signal, query: filter }), [key], { interval: REFRESH });
  // Older pages stay as they were loaded; only the newest page refreshes.
  const [older, setOlder] = useState([]);
  const [ended, setEnded] = useState(false);
  useEffect(() => { setOlder([]); setEnded(false); }, [key]);
  const { busy, error, run } = useAction();
  const first = latest.data || [];
  const list = [...first, ...older.filter(e => !first.some(f => f.id === e.id))];
  const more = useCallback(() => run(async () => {
    const before = list[list.length - 1].id;
    const page = await api.get('/events', { query: { ...filter, before } });
    setOlder(old => [...old, ...page]);
    if (page.length < PAGE) setEnded(true);
  }), [list, key]);
  return html`<section class="section">
    <h2 class="section-title">Events</h2>
    <${Notice} error=${latest.error || error} />
    <div class="notify-filters">
      <select class="input" aria-label="Level" value=${level} onChange=${e => setQuery({ level: e.currentTarget.value })}>
        <option value="">All levels</option>
        ${LEVELS.map(l => html`<option value=${l}>${l[0].toUpperCase() + l.slice(1)}</option>`)}
      </select>
      <select class="input" aria-label="Source" value=${source} onChange=${e => setQuery({ source: e.currentTarget.value })}>
        <option value="">All sources</option>
        ${(sources.data || []).map(s => html`<option value=${s}>${s}</option>`)}
      </select>
      <input class="input" type="search" placeholder="Search" aria-label="Search" value=${search} onInput=${e => setQuery({ q: e.currentTarget.value })} />
    </div>
    ${!latest.data ? (latest.error ? null : html`<${Loading} />`) : list.length === 0 ? html`<${Empty}>${level || source || search ? 'No event matches the filter.' : 'Nothing has happened yet.'}<//>` : html`
      <${Table}>
        <thead><tr><th>Level</th><th>Event</th><th>Source</th><th>When</th></tr></thead>
        <tbody>${list.map(e => html`<tr key=${e.id}>
          <td>${e.kind === 'resolved' ? html`<${Badge} tone="green">Resolved<//>` : html`<${Level} level=${e.level} />`}</td>
          <td>
            <${Linked} href=${target(e.source, e.link)}>${e.title}<//>
            ${e.body ? html`<div class="cell-sub notify-body">${e.body}</div>` : null}
          </td>
          <td>${e.source}</td>
          <td><${When} at=${e.created_at} /></td>
        </tr>`)}</tbody>
      <//>
      ${first.length === PAGE && !ended ? html`<div class="notify-more"><button class="button" disabled=${busy} onClick=${more}>${busy ? 'Loading…' : 'Older events'}</button></div>` : null}`}
  </section>`;
}

export default function Center() {
  return html`<${Page} title="Notifications" subtitle="What needs attention now, and what has happened">
    <${Issues} />
    <${Events} />
  <//>`;
}
