import { html, useState } from '../lib/html.js';
import { useAction, useBusy } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, Field, Check, Badge, confirmDanger, toast, toastError } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, ttl } from '../domains.js';

const TYPES = ['A', 'AAAA', 'CNAME', 'TXT', 'MX', 'NS', 'SRV', 'CAA'];
const TTLS = [[1, 'Auto'], [60, '1 min'], [300, '5 min'], [600, '10 min'], [1800, '30 min'], [3600, '1 h'], [86400, '1 day']];
const PROXIABLE = new Set(['A', 'AAAA', 'CNAME']);

// short shows a record's name relative to the zone: "@" for the apex.
const short = (name, zone) => (name === zone ? '@' : name.endsWith('.' + zone) ? name.slice(0, -zone.length - 1) : name);

export default function Zone({ params }) {
  const { api, href, query, setQuery } = usePanel();
  const base = `/accounts/${params.account}/zones/${encodeURIComponent(params.zone)}`;
  const zone = useApi(base);
  const [editing, setEditing] = useState(null); // a record, or {} for a new one
  const [busy, run] = useBusy(toastError);
  const z = zone.data;
  if (!z) {
    return html`<${Page} title="Domain" back=${{ href: href('/'), label: 'Domains' }}>
      ${zone.error ? html`<${Notice} error=${zone.error} />` : html`<${Loading} />`}<//>`;
  }
  const type = query.type || '';
  const text = (query.q || '').trim().toLowerCase();
  const records = z.records.filter(r => (!type || r.type === type) && (!text || r.name.includes(text) || r.content.toLowerCase().includes(text)));
  const remove = r => {
    if (!confirmDanger(`Delete the ${r.type} record ${short(r.name, z.name)} → ${r.content}?`)) return;
    run(r.id, async () => { await api.del(`${base}/records/${encodeURIComponent(r.id)}`); toast('Record deleted', 'success'); zone.reload(); });
  };
  return html`<${Page} title=${z.name} back=${{ href: href('/'), label: 'Domains' }} subtitle=${z.account_name}
    actions=${editing ? null : html`<button class="button button-primary" onClick=${() => setEditing({})}>Add record</button>`}>
    ${editing ? html`<${RecordForm} key=${editing.id || 'new'} base=${base} zone=${z} record=${editing}
      onDone=${saved => { setEditing(null); if (saved) zone.reload(); }} />` : null}
    <div class="toolbar">
      <select class="input" value=${type} aria-label="Type" style="max-width:140px" onChange=${e => setQuery({ type: e.currentTarget.value })}>
        <option value="">All types</option>${TYPES.map(t => html`<option key=${t} value=${t}>${t}</option>`)}
      </select>
      <input class="input" type="search" placeholder="Filter by name or content" aria-label="Filter" value=${query.q || ''}
        onInput=${e => setQuery({ q: e.currentTarget.value })} />
    </div>
    ${records.length === 0 ? html`<${Empty}>${z.records.length ? 'No record matches.' : 'No records.'}<//>` : html`
      <${Table}>
        <thead><tr><th>Type</th><th>Name</th><th>Content</th><th>TTL</th>${z.proxy ? html`<th>Proxy</th>` : null}<th class="actions"></th></tr></thead>
        <tbody>${records.map(r => html`<tr key=${r.id}>
          <td><${Badge}>${r.type}<//></td>
          <td class="mono">${short(r.name, z.name)}</td>
          <td class="mono content-cell">${r.priority != null && (r.type === 'MX' || r.type === 'SRV') ? `${r.priority} ` : ''}${r.content}
            ${r.comment ? html`<div class="cell-sub">${r.comment}</div>` : null}</td>
          <td class="nowrap">${ttl(r.ttl)}</td>
          ${z.proxy ? html`<td>${r.proxied ? html`<span class="proxied" title="Proxied through Cloudflare">● Proxied</span>` : html`<span class="muted">DNS only</span>`}</td>` : null}
          <td class="actions"><span class="row-actions">
            <button class="button button-small" onClick=${() => setEditing(r)}>Edit</button>
            <button class="button button-small button-danger" disabled=${busy[r.id]} onClick=${() => remove(r)}>Delete</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}

// RecordForm adds or edits a record, inline above the list: a handful of
// fields, so no page or dialog of its own.
function RecordForm({ base, zone, record, onDone }) {
  const { api } = usePanel();
  const isNew = !record.id;
  const [r, setR] = useState({
    type: record.type || 'A', name: record.id ? short(record.name, zone.name) : '', content: record.content || '',
    ttl: record.ttl || 1, priority: record.priority ?? 10, proxied: Boolean(record.proxied), comment: record.comment || '',
  });
  const save = useAction();
  const set = patch => setR(prev => ({ ...prev, ...patch }));
  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      const body = { type: r.type, name: r.name.trim() || '@', content: r.content.trim(), ttl: Number(r.ttl), comment: r.comment.trim() };
      if (r.type === 'MX' || r.type === 'SRV') body.priority = Number(r.priority);
      if (zone.proxy && PROXIABLE.has(r.type)) body.proxied = r.proxied;
      if (isNew) await api.post(`${base}/records`, body);
      else await api.put(`${base}/records/${encodeURIComponent(record.id)}`, body);
      toast(isNew ? 'Record added' : 'Record saved', 'success');
      onDone(true);
    });
  };
  const placeholder = { A: '192.0.2.10', AAAA: '2001:db8::10', CNAME: 'target.example.com', TXT: 'v=spf1 -all', MX: 'mail.example.com',
    NS: 'ns1.example.com', SRV: '10 5060 sip.example.com', CAA: '0 issue "letsencrypt.org"' }[r.type];
  return html`<form class="card record-form" onSubmit=${submit}>
    <h2>${isNew ? 'Add record' : `Edit ${record.type} ${short(record.name, zone.name)}`}</h2>
    <${Notice} error=${save.error} />
    <div class="record-grid">
      <${Field} label="Type">
        <select class="input" value=${r.type} disabled=${!isNew} onChange=${e => set({ type: e.currentTarget.value })}>
          ${TYPES.map(t => html`<option key=${t} value=${t}>${t}</option>`)}
        </select>
      <//>
      <${Field} label="Name" hint=${`@ for ${zone.name}`}>
        <input class="input mono" value=${r.name} placeholder="www" autofocus onInput=${e => set({ name: e.currentTarget.value })} />
      <//>
      <${Field} label="Content">
        <input class="input mono" required value=${r.content} placeholder=${placeholder} onInput=${e => set({ content: e.currentTarget.value })} />
      <//>
      <${Field} label="TTL">
        <select class="input" value=${r.ttl} onChange=${e => set({ ttl: Number(e.currentTarget.value) })}>
          ${TTLS.map(([v, l]) => html`<option key=${v} value=${v}>${l}</option>`)}
          ${TTLS.some(([v]) => v === r.ttl) ? null : html`<option value=${r.ttl}>${ttl(r.ttl)}</option>`}
        </select>
      <//>
    </div>
    <div class="button-actions">
      ${r.type === 'MX' || r.type === 'SRV' ? html`<${Field} label="Priority"><input class="input" type="number" min="0" max="65535" style="width:110px"
        value=${r.priority} onInput=${e => set({ priority: e.currentTarget.value })} /><//>` : null}
      ${zone.proxy && PROXIABLE.has(r.type) ? html`<${Check} label="Proxy through Cloudflare (hides the address; HTTP and HTTPS only)"
        checked=${r.proxied} onChange=${e => set({ proxied: e.currentTarget.checked })} />` : null}
    </div>
    <${Field} label="Comment"><input class="input" value=${r.comment} onInput=${e => set({ comment: e.currentTarget.value })} /><//>
    <div class="button-actions">
      <button class="button button-primary" disabled=${save.busy}>${isNew ? 'Add' : 'Save'}</button>
      <button class="button" type="button" onClick=${() => onDone(false)}>Cancel</button>
    </div>
  </form>`;
}
