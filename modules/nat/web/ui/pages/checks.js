import { html, useEffect, useRef, useState } from '@lsongdev/panel/html.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { PageHeader, Alert, Field, Loading, Badge, Empty } from '@lsongdev/scripts/react/components/index.js';
import { useApi, date, statuses, verdict } from '../nat.js';

export default function Checks() {
  const { api, href, navigate } = usePanel();
  const defaults = useApi('/defaults');
  const recent = useApi('/checks', 3000);
  const [servers, setServers] = useState('');
  const [local, setLocal] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const initialized = useRef(false);
  const submitting = useRef(false);
  useEffect(() => {
    if (defaults.data && !initialized.current) {
      initialized.current = true;
      setServers(defaults.data.servers.join('\n'));
    }
  }, [defaults.data]);
  const active = recent.data?.find(c => c.status === 'running');
  const run = async e => {
    e.preventDefault();
    if (submitting.current) return;
    submitting.current = true; setBusy(true); setError(null);
    try {
      const check = await api.post('/checks', { servers: servers.split('\n').map(s => s.trim()).filter(Boolean), local_address: local.trim() });
      navigate(`/checks/${check.id}`);
    } catch (err) { setError(err); recent.reload(); }
    finally { submitting.current = false; setBusy(false); }
  };
  return html`
    <${PageHeader} title="NAT detection" subtitle="Understand this server’s public IPv4 connection"
      actions=${html`<button class="button button-primary" form="nat-detection-form" type="submit" disabled=${busy || !!active || !defaults.data}>${busy ? 'Starting…' : 'Run detection'}</button>`} />
    <div class="nat-landing">
      <${Alert} error=${defaults.error || recent.error} />
      ${active ? html`<div class="nat-active" role="status"><span class="nat-dot is-pending" aria-hidden="true"></span><span>A detection is running in the background.</span><a href=${href(`/checks/${active.id}`)}>View progress →</a></div>` : null}
      <section class="nat-setup" aria-labelledby="nat-setup-title">
        <div class="nat-setup-heading"><span class="nat-eyebrow">IPv4 / UDP</span><h2 id="nat-setup-title">Detection settings</h2><p>Find your public endpoint, then check how mappings and incoming replies behave.</p></div>
        ${!defaults.data ? (defaults.error ? null : html`<${Loading} />`) : html`
          <form id="nat-detection-form" class="form nat-form" onSubmit=${run}>
            <${Field} label="STUN servers" required hint="One host:port per line. Use up to five servers." error=${error?.fields?.servers}>
              <textarea class="input mono" rows="3" required value=${servers} disabled=${busy} onInput=${e => setServers(e.currentTarget.value)} spellcheck="false" />
            <//>
            <${Field} label="Local IPv4 address" hint="Leave automatic to use this server’s default route." error=${error?.fields?.local_address}>
              <select class="input mono" value=${local} disabled=${busy} onChange=${e => setLocal(e.currentTarget.value)}>
                <option value="">Automatic · Default route</option>
                ${defaults.data.addresses.map(a => html`<option key=${a} value=${a}>${a}</option>`)}
              </select>
            <//>
            <${Alert} error=${error} />
          </form>`}
        <div class="nat-setup-footnote">Full behavior detection requires a compatible STUN server. This check does not establish a tunnel or verify inbound service access.</div>
      </section>
      <section class="nat-history" aria-labelledby="nat-history-title">
        <div class="nat-section-heading"><h2 id="nat-history-title">Recent detections</h2>${recent.data?.length ? html`<span class="muted small">${recent.data.length} / 20 retained</span>` : null}</div>
        <p class="muted small nat-retention">Results remain available until this process restarts.</p>
        ${!recent.data ? (recent.error ? null : html`<${Loading} />`) : !recent.data.length ? html`<${Empty}><strong>No detections yet</strong><p>Run a detection to see your network behavior and the evidence behind it.</p><//>` : html`
          <ol class="nat-history-list">${recent.data.map(c => html`<li key=${c.id}><a class="nat-history-link" href=${href(`/checks/${c.id}`)}>
            <div class="nat-history-main"><div class="nat-history-meta"><time datetime=${c.started}>${date(c.started)}</time><${Badge} variant=${c.status === 'running' ? 'info' : ['cancelled', 'timed_out'].includes(c.status) ? 'warning' : undefined}>${statuses[c.status] || c.status}</${Badge}></div><h3>${verdict(c).title}</h3><span class="mono muted">${c.result.public?.join(' · ') || 'Public endpoint not discovered'}</span></div>
            <span class="nat-history-arrow" aria-hidden="true">↗</span>
          </a></li>`)}</ol>`}
      </section>
    </div>
  `;
}
