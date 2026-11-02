import { html, useEffect, useState } from '../lib/html.js';
import { useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, Check, LogView } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';

export default function Logs() {
  const { query, setQuery } = usePanel();
  const [search, setSearch] = useState(query.q || '');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => setSearch(query.q || ''), [query.q]);
  const live = query.live !== '0';
  const limit = query.limit || '500';
  return html`<${Page} title="Logs" subtitle="smbd.log" actions=${html`<button type="button" class="button" onClick=${() => setRefresh(n => n + 1)}>Refresh</button>`}>
    <form class="samba-log-filters" onSubmit=${e => { e.preventDefault(); setQuery({ q: search.trim() }); }}>
        <${Field} label="Search recent entries">
          <input class="input" type="search" value=${search} maxlength="256" placeholder="IP, username, denied, NT_STATUS…"
            onInput=${e => setSearch(e.currentTarget.value)} />
        <//>
        <${Field} label="Entries">
          <select class="input" value=${limit} onChange=${e => setQuery({ limit: e.currentTarget.value })}>
            ${['200', '500', '1000', '5000'].map(n => html`<option value=${n}>${n}</option>`)}
          </select>
        <//>
        <button class="button" type="submit">Search</button>
        <${Check} label="Auto-refresh (5s)" checked=${live} onChange=${e => setQuery({ live: e.currentTarget.checked ? '' : '0' })} />
      </form>
      <${LogOutput} key=${JSON.stringify([query.q, limit])} search=${query.q || ''} limit=${limit} live=${live} refresh=${refresh} />
  <//>`;
}

function LogOutput({ search, limit, live, refresh }) {
  const { api } = usePanel();
  const logs = useResource(signal => api.get('/logs', { signal, query: { q: search, limit } }), [search, limit, refresh], { interval: live ? 5000 : 0 });
  const data = logs.data;
  // A failed poll leaves the last readable output visible.
  const lines = (data?.entries || []).map(text => /denied|NT_STATUS_|authentication.*fail|permission|error|failed/i.test(text) ? { text, stream: 'stderr' } : text);
  return html`<section class="samba-logs" aria-label="Samba log output">
    <${Notice} error=${logs.error} />
    ${!data ? (logs.error ? null : html`<${Loading} />`) : html`
      <${LogView} lines=${lines} follow=${live} className="samba-log-output" empty=${search ? 'No matching entries in the recent log window. Try a broader search.' : 'No entries in this log yet. Retry the connection with auto-refresh enabled.'} />
      <p class="muted small samba-log-status" role="status">
        <code>${data.path}</code> · ${data.entries.length} entries · ${live ? 'Auto-refresh on' : 'Auto-refresh paused'} · Read at ${new Date(data.read_at).toLocaleTimeString()}
      </p>
      ${data.truncated ? html`<p class="muted small">Showing recent entries from the latest 1 MiB of smbd.log. Older matches may be omitted.</p>` : null}
    `}
  </section>`;
}
