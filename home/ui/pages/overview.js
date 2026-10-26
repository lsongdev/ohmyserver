// The overview links into other modules with ctx.link, which returns null
// for modules that are not mounted, so this page adapts to the build.
import { html, useMemo } from '../../../../_shell/html.js';
import { useResource } from '../../../../_shell/hooks.js';
import { createClient } from '../../../../_shell/client.js';
import { Page, Notice, Loading, Status, Badge, Table } from '../../../../_shell/ui.js';
import { bytes } from '../../../../_shell/format.js';

function Card({ title, to, value, sub, error }) {
  const body = html`<span class="stat-label">${title}</span>
    ${error ? html`<span class="stat-sub" style="color:var(--danger)">${error}</span>` : html`
      <span class="stat-value">${value}</span>
      ${sub ? html`<span class="stat-sub">${sub}</span>` : null}`}`;
  return to ? html`<a class="card stat" href=${to}>${body}</a>` : html`<div class="card stat">${body}</div>`;
}

export default function Overview({ ctx }) {
  const api = useMemo(() => createClient(ctx.apiBase), [ctx.apiBase]);
  const summary = useResource(signal => api.get('summary', { signal }), [api], { interval: 10000 });
  const s = summary.data;
  const link = ctx.link;
  const local = s?.monitor?.local?.latest;
  const pct = (used, total) => (total ? Math.round((used / total) * 100) : 0);
  return html`<${Page} title="Overview">
    <${Notice} error=${summary.error} />
    ${!s ? (summary.error ? null : html`<${Loading} />`) : html`
      ${local ? html`<div class="grid" style="margin-bottom:16px">
        <${Card} title="CPU" to=${link('monitor', '/hosts/local')} value=${`${local.cpu.toFixed(1)}%`} sub=${`Load ${local.load1.toFixed(2)}`} />
        <${Card} title="Memory" to=${link('monitor', '/hosts/local')} value=${`${pct(local.mem_used, local.mem_total)}%`}
          sub=${`${bytes(local.mem_used)} / ${bytes(local.mem_total)}`} />
        <${Card} title="Disk" to=${link('monitor', '/hosts/local')} value=${`${pct(local.disk_used, local.disk_total)}%`}
          sub=${`${bytes(local.disk_used)} / ${bytes(local.disk_total)}`} />
        ${s.monitor.hosts > 1 ? html`<${Card} title="Monitored hosts" to=${link('monitor', '/')} value=${`${s.monitor.online} / ${s.monitor.hosts}`}
          sub=${s.monitor.offline ? `${s.monitor.offline} offline` : 'All online'} />` : null}
      </div>` : null}
      <div class="grid">
        ${s.docker ? html`
          <${Card} title="Containers" to=${link('docker', '/containers')} error=${s.docker.error}
            value=${s.docker.containers} sub=${`${s.docker.running} running`} />
          <${Card} title="Apps" to=${link('docker', '/apps')}
            value=${s.docker.apps} sub=${`${s.docker.apps_running} running`} />` : null}
        ${s.certificates ? html`
          <${Card} title="Certificates" to=${link('acme', '/certificates')} error=${s.certificates.error}
            value=${s.certificates.active}
            sub=${s.certificates.expiring ? `${s.certificates.expiring} expiring within 30 days` : s.certificates.pending ? `${s.certificates.pending} being issued` : 'Valid certificates'} />` : null}
        ${s.gateway ? html`
          <${Card} title="Gateway sites" to=${link('yabber', '/sites')}
            value=${s.gateway.sites} sub=${`${s.gateway.requests} requests`} />` : null}
        ${s.dns ? html`
          <${Card} title="DNS queries" to=${link('dns', '/')}
            value=${s.dns.queries} sub=${`${s.dns.blocked} blocked · ${s.dns.zones} zones`} />` : null}
        ${s.forward ? html`
          <${Card} title="Port forwards" to=${link('forward', '/')}
            value=${s.forward.running}
            sub=${s.forward.failed ? `${s.forward.failed} failed to start` : `${s.forward.rules} rules · ${s.forward.active} connections`} />` : null}
        ${s.cron ? html`
          <${Card} title="Scheduled jobs" to=${link('cron', '/')}
            value=${s.cron.enabled}
            sub=${s.cron.failing ? `${s.cron.failing} failed last run` : s.cron.running ? `${s.cron.running} running` : `${s.cron.jobs} jobs`} />` : null}
        ${s.ci ? html`
          <${Card} title="CI" to=${link('ci', '/')} value=${s.ci.repos}
            sub=${s.ci.running || s.ci.pending ? `${s.ci.running} building · ${s.ci.pending} queued` : s.ci.failing ? `${s.ci.failing} repositories failing` : 'All latest builds passed'} />` : null}
        ${s.ssh ? html`
          <${Card} title="SSH hosts" to=${link('ssh', '/')}
            value=${s.ssh.hosts} sub=${`${s.ssh.sessions} open terminals`} />` : null}
      </div>
      ${s.gateway ? html`<div class="section"><h2>Gateway listeners</h2>
        <${Table}>
          <thead><tr><th>Name</th><th>Address</th><th>TLS</th><th>Status</th></tr></thead>
          <tbody>${s.gateway.listeners.map(l => html`<tr key=${l.name}>
            <td>${l.name}</td><td class="mono">${l.address}</td><td>${l.tls ? html`<${Badge} tone="blue">TLS<//>` : '—'}</td>
            <td>${l.running ? html`<${Status} state="running" />` : html`<${Status} state="failed" label=${l.error || 'Not running'} />`}</td>
          </tr>`)}</tbody>
        <//>
      </div>` : null}
      ${s.dns ? html`<div class="section"><h2>DNS listeners</h2>
        <${Table}>
          <thead><tr><th>Protocol</th><th>Address</th><th>TLS</th><th>Status</th></tr></thead>
          <tbody>${s.dns.listeners.map(l => html`<tr key=${l.type + l.addr}>
            <td>${l.type.toUpperCase()}</td><td class="mono">${l.addr}</td><td>${l.tls ? html`<${Badge} tone="blue">TLS<//>` : '—'}</td>
            <td>${l.error ? html`<${Status} state="failed" label=${l.error} />` : html`<${Status} state="running" />`}</td>
          </tr>`)}</tbody>
        <//>
      </div>` : null}`}
  <//>`;
}
