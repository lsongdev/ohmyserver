import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, Tabs, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { bytes, datetime, ago } from '../lib/format.js';
import { useApi, ServiceStatus, Journal, Unsupported, isActive, unitLabel } from '../services.js';

export default function Service({ params }) {
  const { api, query, setQuery } = usePanel();
  const name = params.name;
  const path = `/services/${encodeURIComponent(name)}`;
  const service = useApi(path, 5000);
  const action = useAction();
  const tab = query.tab || 'logs';
  const title = unitLabel(name);

  if (!service.data) {
    return html`<${Page} title=${title}>
      ${service.error?.code === 'unsupported' ? html`<${Unsupported} />` : service.error ? html`<${Notice} error=${service.error} />` : html`<${Loading} />`}
    <//>`;
  }
  const s = service.data;
  const run = (verb, confirm) => {
    if (confirm && !window.confirm(confirm)) return;
    action.run(async () => {
      await api.post(`${path}/${verb}`);
      toast(`${title}: ${verb} done`, 'success');
      service.reload();
    });
  };
  const active = isActive(s);
  const buttons = [];
  if (active) {
    buttons.push(html`<button class="button" disabled=${action.busy} onClick=${() => run('restart', s.self
      ? 'Restart the panel? This page reconnects once it is back.' : `Restart ${title}?`)}>Restart</button>`);
    buttons.push(html`<button class="button" disabled=${action.busy} onClick=${() => run('reload', null)}
      title="Ask the service to reload its configuration, if it supports that">Reload</button>`);
    if (!s.self) buttons.push(html`<button class="button button-danger" disabled=${action.busy} onClick=${() => run('stop', `Stop ${title}?`)}>Stop</button>`);
  } else if (s.load === 'loaded' && s.enabled !== 'masked' && !s.guarded) {
    // Static units usually start as part of something else: ask first.
    const ask = s.enabled === 'static' ? `${title} is usually started by the system, not by hand. Start it anyway?` : null;
    buttons.push(html`<button class="button button-primary" disabled=${action.busy} onClick=${() => run('start', ask)}>Start</button>`);
  }
  if (s.enabled === 'enabled' && !s.self) {
    buttons.push(html`<button class="button" disabled=${action.busy} onClick=${() => run('disable', `Stop starting ${title} at boot?`)}>Disable</button>`);
  } else if (s.enabled === 'disabled') {
    buttons.push(html`<button class="button" disabled=${action.busy} onClick=${() => run('enable', null)}>Enable</button>`);
  }
  const exit = s.result && s.result !== 'success' ? `${s.result}${s.exit_status ? ` (exit status ${s.exit_status})` : ''}` : null;

  return html`<${Page} title=${title} subtitle=${s.description}
    actions=${html`<span class="page-actions">${buttons}</span>`}>
    ${s.guarded ? html`<${Notice} tone="info">Starting this service shuts down, reboots or switches the system to rescue mode, so the panel leaves it to a shell.<//>` : null}
    ${s.self ? html`<${Notice} tone="info">This service runs the panel. It can be restarted from here; stop or disable it from a shell.<//>` : null}
    <${Notice} error=${action.error} />
    <div class="card">
      <${KV} items=${[
        ['Status', html`<${ServiceStatus} unit=${s} />`],
        [active ? 'Running since' : 'Stopped since', s.since ? html`${datetime(s.since)} <span class="muted">(${ago(s.since)})</span>` : null],
        ['Start at boot', s.enabled ? `${s.enabled}${s.preset ? ` (preset: ${s.preset})` : ''}` : null],
        ['Main process', s.main_pid ? String(s.main_pid) : null],
        ['Memory', s.memory ? bytes(s.memory) : null],
        ['CPU time', s.cpu_ns ? `${(s.cpu_ns / 1e9).toFixed(1)}s` : null],
        ['Tasks', s.tasks ? String(s.tasks) : null],
        ['Restarts', s.restarts ? String(s.restarts) : null],
        ['Last result', exit],
        ['Unit file', s.path ? html`<code>${s.path}</code>` : null],
      ]} />
    </div>
    <section class="section">
      <${Tabs} active=${tab} onChange=${id => setQuery({ tab: id === 'logs' ? '' : id })} tabs=${[['logs', 'Logs'], ['file', 'Unit file']]} />
      ${tab === 'file' ? html`<${UnitFile} path=${path} />` : html`<${Journal} query=${{ unit: name, lines: 300 }} live showUnit=${false} />`}
    </section>
  <//>`;
}

function UnitFile({ path }) {
  const file = useApi(`${path}/file`);
  if (!file.data) return file.error ? html`<${Notice} error=${file.error} />` : html`<${Loading} />`;
  return html`<pre class="card unit-file">${file.data.text || 'No unit file'}</pre>`;
}
