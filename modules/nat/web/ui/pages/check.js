import { html, useState } from '@lsongdev/panel/html.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { PageHeader, Alert, Loading, Badge, toast } from '@lsongdev/scripts/react/components/index.js';
import { useApi, date, mapping, filtering, statuses, steps, verdict, mappingHint, filteringHint, elapsed } from '../nat.js';

function Evidence({ records }) {
  const groups = [
    { title: 'STUN connectivity', description: 'Discover the public endpoint seen by each server.', includes: s => s.name === 'resolve' || s.name === 'binding' },
    { title: 'Mapping behavior', description: 'Compare mappings while changing the destination.', includes: s => s.name === 'behavior_baseline' || s.name.startsWith('mapping_') },
    { title: 'Filtering behavior', description: 'Test replies from alternate endpoints, then verify with control requests.', includes: s => s.name.startsWith('filter_') },
  ];
  const responses = records.filter(s => s.response).length;
  return html`<details class="nat-disclosure nat-evidence-section">
    <summary><span><strong>Detailed evidence</strong><span class="nat-summary-caption">${records.length} steps · ${responses} responses</span></span><span class="nat-disclosure-action" aria-hidden="true">Expand</span></summary>
    <div class="nat-disclosure-body">
      <p class="muted small">A CHANGE-REQUEST timeout can be expected during filtering tests. It is interpreted together with the control requests.</p>
      ${groups.map(group => {
        const items = records.map((step, index) => ({ ...step, index })).filter(group.includes);
        return items.length ? html`<section class="nat-evidence-group" key=${group.title}>
          <h3>${group.title}</h3><p class="muted small">${group.description}</p>
          <ol class="nat-evidence">${items.map(s => html`<li key=${s.index}><details>
            <summary class="nat-step-heading">
              <span class="nat-step-number" aria-hidden="true">${String(s.index + 1).padStart(2, '0')}</span>
              <span class="nat-step-copy"><strong>${steps[s.name] || s.name}</strong><span class="mono muted">${s.server}</span></span>
              <span class="nat-step-outcome"><${Badge} variant=${s.error ? s.timeout ? 'warning' : 'danger' : 'success'}>${s.error ? s.timeout ? 'No reply' : 'Failed' : 'Received'}</${Badge}><span class="muted small">${elapsed(s.duration_ms)}</span></span>
            </summary>
            <div class="nat-step-body"><dl class="nat-step">
              <div><dt>Sent to</dt><dd class="mono">${s.target || '—'}</dd></div>
              <div><dt>Expected reply from</dt><dd class="mono">${s.expected_source || '—'}</dd></div>
              <div><dt>Local socket</dt><dd class="mono">${s.local || '—'}</dd></div>
              ${s.response ? html`
                <div><dt>Reply from</dt><dd class="mono">${s.response.source}</dd></div>
                <div><dt>Mapped address</dt><dd class="mono">${s.response.mapped}</dd></div>
                <div><dt>Server origin</dt><dd class="mono">${s.response.origin || '—'}</dd></div>
                <div><dt>Alternate endpoint</dt><dd class="mono">${s.response.other || '—'}</dd></div>` : null}
            </dl>${s.error ? html`<p class="nat-step-error">${s.error}</p>` : null}</div>
          </details></li>`)}</ol>
        </section>` : null;
      })}
      ${!records.length ? html`<p class="muted">No steps recorded yet.</p>` : null}
    </div>
  </details>`;
}

export default function Check({ params }) {
  const { api, href } = usePanel();
  const check = useApi(`/checks/${encodeURIComponent(params.id)}`, data => (data?.status === 'running' ? 1000 : 0));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const c = check.data;
  const stop = async () => {
    if (busy) return;
    setBusy(true); setError(null);
    try { await api.post(`/checks/${c.id}/cancel`); check.reload(); }
    catch (err) { setError(err); } finally { setBusy(false); }
  };
  const download = () => {
    const url = URL.createObjectURL(new Blob([JSON.stringify(c, null, 2)], { type: 'application/json' }));
    const a = document.createElement('a'); a.href = url; a.download = `nat-check-${c.id}.json`; a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000); toast('Detection report downloaded', 'success');
  };
  const r = c?.result;
  const outcome = c && verdict(c);
  const running = c?.status === 'running';
  const partial = c && ['cancelled', 'timed_out'].includes(c.status);
  const records = r?.steps || [];
  const reached = new Set(records.filter(s => s.name === 'binding' && s.response).map(s => s.server)).size;
  const fakeIP = r?.notes?.some(n => n.includes('possible Fake-IP'));
  const stage = records.some(s => s.name.startsWith('filter_')) ? 2 : records.some(s => s.name === 'behavior_baseline' || s.name.startsWith('mapping_')) ? 1 : 0;
  return html`
    <${PageHeader} title="NAT detection" subtitle=${c ? date(c.started) : 'Detection details'}
      back=${!c ? { href: href('/'), label: 'All checks' } : null}
      actions=${c ? html`
        ${running ? html`<button class="button" disabled=${busy} onClick=${stop}>${busy ? 'Stopping…' : 'Stop detection'}</button>` : html`<button class="button" onClick=${download}>Download report</button>`}
        <a class="button ${running ? '' : 'button-primary'}" href=${href('/')}>${running ? 'All checks' : 'New detection'}</a>
      ` : null} />
    <${Alert} error=${check.error || error} />
    ${check.error ? html`<button class="button" onClick=${check.reload}>Retry loading</button>` : null}
    ${!c ? (check.error ? null : html`<${Loading} />`) : html`<div class="nat-report">
      <section class=${`nat-verdict nat-tone-${outcome.tone}`} aria-labelledby="nat-result-title">
        <div class="nat-verdict-top"><span class="nat-eyebrow">IPv4 / UDP</span><${Badge} variant=${running ? 'info' : partial ? 'warning' : undefined}>${statuses[c.status]}</${Badge}></div>
        <h2 id="nat-result-title">${outcome.title}</h2>
        <p class="nat-verdict-description">${outcome.description}</p>
        <p class="nat-scope">${partial ? 'Partial results. ' : ''}Network behavior only · Peer-to-peer connectivity has not been verified</p>
        ${running ? html`<div class="nat-progress" role="status">
          <progress aria-label="NAT detection in progress"></progress><span>${records.length} steps recorded · Up to 60 seconds</span>
          <ol class="nat-stages" aria-label="Detection stages">${['STUN connectivity', 'Mapping', 'Filtering'].map((label, i) => html`<li key=${label} class=${i === stage ? 'is-current' : i < stage ? 'is-previous' : ''} aria-current=${i === stage ? 'step' : undefined}><span>${i + 1}</span>${label}</li>`)}</ol>
        </div>` : null}
      </section>

      <section aria-label="Key findings" class="nat-findings">
        <article class="nat-finding"><div class="nat-finding-label"><span class=${`nat-dot ${r.udp ? 'is-positive' : running ? 'is-pending' : 'is-unknown'}`} aria-hidden="true"></span>UDP connectivity</div><h3>${r.udp ? 'STUN reachable' : running ? 'Checking…' : 'Not verified'}</h3><p>${r.udp ? `${reached} of ${c.config.servers.length} configured servers replied.` : 'A binding response is needed to discover the public endpoint.'}</p></article>
        <article class="nat-finding"><div class="nat-finding-label">Mapping behavior</div><h3>${running && r.mapping === 'unknown' ? 'Checking…' : mapping[r.mapping] || 'Unknown'}</h3><p>${running && r.mapping === 'unknown' ? 'Comparing public endpoints across tested destinations.' : mappingHint[r.mapping] || mappingHint.unknown}</p></article>
        <article class="nat-finding"><div class="nat-finding-label">Filtering behavior</div><h3>${running && r.filtering === 'unknown' ? stage < 2 ? 'Waiting for mapping' : 'Checking…' : filtering[r.filtering] || 'Unknown'}</h3><p>${running && r.filtering === 'unknown' ? 'Testing whether alternate endpoints can send replies.' : filteringHint[r.filtering] || filteringHint.unknown}</p></article>
      </section>

      <section class="nat-path-section" aria-labelledby="nat-path-title">
        <div class="nat-section-heading"><h2 id="nat-path-title">Observed network path</h2><span class="muted small">From this server</span></div>
        <div class="nat-path">
          <div class="nat-path-node"><span class="nat-node-label">Local socket</span><strong>This server</strong><span class="mono">${r.local || 'Waiting for socket…'}</span>${r.local?.startsWith('0.0.0.0:') ? html`<span class="muted small">Automatic source address</span>` : null}</div>
          <span class="nat-path-arrow" aria-hidden="true">→</span>
          <div class="nat-path-node nat-path-public"><span class="nat-node-label">Observed public endpoint</span><strong>${r.public?.length ? r.public.length === 1 ? 'Public mapping' : 'Multiple mappings' : 'Not discovered'}</strong>${r.public?.length ? r.public.map(p => html`<span class="mono" key=${p}>${p}</span>`) : html`<span class="muted small">Awaiting a STUN response</span>`}</div>
          <span class="nat-path-arrow" aria-hidden="true">→</span>
          <div class="nat-path-node"><span class="nat-node-label">External observation</span><strong>STUN servers</strong><span class="muted small">${reached} responding / ${c.config.servers.length} configured</span></div>
        </div>
        ${fakeIP ? html`<div class="nat-path-note"><span class="nat-note-marker" aria-hidden="true">i</span><div><strong>Possible proxy path</strong><p>DNS returned Fake-IP-style addresses. These observations include the current gateway or proxy behavior.</p></div></div>` : null}
      </section>

      <div class="nat-section-heading nat-details-heading"><h2>Technical details</h2><span class="muted small">For diagnosis and verification</span></div>
      ${r.notes?.length ? html`<details class="nat-disclosure"><summary><span><strong>Notes and limitations</strong><span class="nat-summary-caption">${r.notes.length} observations · IPv4 UDP only</span></span><span class="nat-disclosure-action" aria-hidden="true">Expand</span></summary><div class="nat-disclosure-body"><ul class="nat-notes">${r.notes.map(n => html`<li key=${n}>${n}</li>`)}</ul></div></details>` : null}
      <${Evidence} records=${records} />
    </div>`}
  `;
}
