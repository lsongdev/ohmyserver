import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, Table, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { bytes, datetime } from '../lib/format.js';
import { useApi, kind, uptime, usage, HealthStatus } from '../disks.js';

// Attributes whose raw count means trouble when it is above zero.
const CRITICAL = new Set([5, 187, 197, 198, 199]);

export default function Disk({ params }) {
  const { api, href } = usePanel();
  const path = `/disks/${encodeURIComponent(params.name)}`;
  // Poll faster while a self-test runs.
  const disk = useApi(path, d => (d?.health?.testing != null ? 15000 : 60000));
  const action = useAction();
  const back = { href: href('/'), label: 'Storage' };
  if (!disk.data) {
    return html`<${Page} title=${params.name} back=${back}>${disk.error ? html`<${Notice} error=${disk.error} />` : html`<${Loading}>Reading the disk…<//>`}<//>`;
  }
  const d = disk.data, h = d.health || {}, r = d.report;
  const test = (kind, confirm) => {
    if (confirm && !window.confirm(confirm)) return;
    action.run(async () => {
      await api.post(`${path}/test`, { kind });
      toast(kind === 'abort' ? 'Self-test aborted' : 'Self-test started', 'success');
      disk.reload();
    });
  };
  const wake = () => window.confirm(`Read ${d.name} now? A disk in standby spins up.`) &&
    action.run(async () => { await api.get(path, { query: { wake: 1 } }); disk.reload(); });
  const polling = r?.ata_smart_data?.self_test?.polling_minutes;
  const actions = r && !h.error ? html`<span class="button-row">
    ${h.testing != null ? html`<button class="button" disabled=${action.busy} onClick=${() => test('abort', 'Abort the running self-test?')}>Abort test</button>` : html`
      <button class="button" disabled=${action.busy} onClick=${() => test('short', null)}
        title=${polling?.short ? `About ${polling.short} minutes` : 'A few minutes'}>Short test</button>
      <button class="button" disabled=${action.busy} onClick=${() => test('long', `Run an extended self-test? It reads the whole disk${polling?.extended ? `, about ${Math.round(polling.extended / 60)} hours` : ''}; the disk stays usable meanwhile.`)}>Extended test</button>`}
  </span>` : null;

  return html`<${Page} title=${`${d.name}${d.model ? ` · ${d.model}` : ''}`} back=${back} actions=${actions}>
    <${Notice} error=${action.error} />
    ${h.error ? html`<${Notice} tone="info">No SMART data: ${h.error}<//>` : null}
    ${h.asleep ? html`<${Notice} tone="info">The disk is in standby, so it was left alone${r ? '' : ' and nothing new was read'}.
      <button class="button button-small" style="margin-left:8px" disabled=${action.busy} onClick=${wake}>Read now</button><//>` : null}
    ${h.warnings?.length ? html`<${Notice} tone=${h.passed === false ? 'error' : 'warn'}>
      <ul class="warnings">${h.warnings.map(w => html`<li key=${w}>${w}</li>`)}</ul><//>` : null}
    <div class="grid-2">
      <div class="card"><h2>Disk</h2>
        <${KV} items=${[
          ['Health', html`<${HealthStatus} health=${h} />`],
          ['Model', r?.model_name || d.model],
          ['Family', r?.model_family],
          ['Serial', r?.serial_number || d.serial],
          ['Firmware', r?.firmware_version],
          ['Capacity', bytes(d.size)],
          ['Type', [kind(d), r?.rotation_rate > 0 ? `${r.rotation_rate} rpm` : null, r?.interface_speed?.current?.string].filter(Boolean).join(' · ')],
          ['Used by', usage(d).join(', ') || null],
        ]} />
      </div>
      <div class="card"><h2>Condition</h2>
        <${KV} items=${[
          ['Temperature', h.temperature ? `${h.temperature} °C` : null],
          ['Powered on', h.power_on_hours ? `${uptime(h.power_on_hours)} (${h.power_on_hours.toLocaleString()} hours)` : null],
          ['Power cycles', h.power_cycles ? h.power_cycles.toLocaleString() : null],
          ['Endurance used', h.wear != null ? `${h.wear}%` : null],
          ['Self-test', h.testing != null ? `Running, ${h.testing}% left` : r?.ata_smart_data?.self_test?.status?.string],
          ['Checked', h.checked_at ? datetime(h.checked_at) : null],
        ]} />
      </div>
    </div>
    ${r?.ata_smart_attributes?.table?.length ? html`<section class="section">
      <h2>SMART attributes</h2>
      <${Table}>
        <thead><tr><th class="num">ID</th><th>Attribute</th><th class="num">Value</th><th class="num">Worst</th><th class="num">Threshold</th><th class="num">Raw</th><th>Failed</th></tr></thead>
        <tbody>${r.ata_smart_attributes.table.map(a => html`<tr key=${a.id} class=${(CRITICAL.has(a.id) && a.raw.value > 0) || a.when_failed ? 'attr-alert' : ''}>
          <td class="num">${a.id}</td><td>${a.name.replaceAll('_', ' ')}</td>
          <td class="num">${a.value}</td><td class="num">${a.worst}</td><td class="num">${a.thresh}</td>
          <td class="num">${a.raw.string}</td><td>${a.when_failed || '—'}</td>
        </tr>`)}</tbody>
      <//>
    </section>` : null}
    ${r?.nvme_smart_health_information_log && r.nvme_version?.string ? html`<section class="section">
      <h2>NVMe health</h2>
      <div class="card"><${KV} items=${Object.entries(r.nvme_smart_health_information_log).map(([k, v]) => [k.replaceAll('_', ' '), String(v)])} /></div>
    </section>` : null}
    ${r?.ata_smart_self_test_log?.standard?.table?.length ? html`<section class="section">
      <h2>Self-test log</h2>
      <${Table}>
        <thead><tr><th>Test</th><th>Result</th><th class="num">At hour</th></tr></thead>
        <tbody>${r.ata_smart_self_test_log.standard.table.map((t, i) => html`<tr key=${i}>
          <td>${t.type.string}</td><td class=${t.status.passed ? '' : 'text-error'}>${t.status.string}</td><td class="num">${t.lifetime_hours.toLocaleString()}</td>
        </tr>`)}</tbody>
      <//>
    </section>` : null}
  <//>`;
}
