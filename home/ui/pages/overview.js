// The overview links into other modules with ctx.link, which returns null
// for modules that are not mounted, so this page adapts to the build.
import { html, useMemo } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { createClient } from '@lsongdev/panel/client.js';
import { PageHeader, Alert, Loading, Badge, Table } from '@lsongdev/scripts/react/components/index.js';
import { Status } from '@lsongdev/panel/ui.js';
import { bytes } from '@lsongdev/panel/format.js';

function Card({ title, to, value, sub, error }) {
  const body = html`<span class="stat-label">${title}</span>
    ${error ? html`<span class="stat-sub" style="color:var(--color-danger)">${error}</span>` : html`
      <span class="stat-value">${value}</span>
      ${sub ? html`<span class="stat-sub">${sub}</span>` : null}`}`;
  return to ? html`<a class="card stat" href=${to}>${body}</a>` : html`<div class="card stat">${body}</div>`;
}

// Issues lists what is wrong right now, and takes no room when nothing is.
function Issues({ summary: n, link }) {
  if (!n?.open) return null;
  const tones = { critical: 'danger', warning: 'warning', info: 'info' };
  const target = i => {
    if (!i.link) return link('notify', '/');
    const [route, search = ''] = i.link.split('?');
    return link(i.source, route, Object.fromEntries(new URLSearchParams(search))) || link('notify', '/');
  };
  return html`<div class="card" style="margin-bottom:16px">
    <div style="display:flex;justify-content:space-between;gap:8px;flex-wrap:wrap;margin-bottom:10px">
      <strong>${n.open === 1 ? '1 open issue' : `${n.open} open issues`}</strong>
      ${link('notify', '/') ? html`<a href=${link('notify', '/')}>All notifications</a>` : null}
    </div>
    <div style="display:grid;gap:6px">${n.issues.map(i => html`<div key=${i.id} style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
      <${Badge} variant=${tones[i.level]}>${i.level[0].toUpperCase() + i.level.slice(1)}<//>
      ${target(i) ? html`<a href=${target(i)}>${i.title}</a>` : i.title}
    </div>`)}</div>
  </div>`;
}

export default function Overview({ ctx }) {
  const api = useMemo(() => createClient(ctx.apiBase), [ctx.apiBase]);
  const summary = useResource(signal => api.get('summary', { signal }), [api], { interval: 10000 });
  const s = summary.data;
  const link = ctx.link;
  const local = s?.monitor?.local?.latest;
  const pct = (used, total) => (total ? Math.round((used / total) * 100) : 0);
  return html`<${PageHeader} title="Dashboard" />
    <${Alert} error=${summary.error} />
    ${!s ? (summary.error ? null : html`<${Loading} />`) : html`
      <${Issues} summary=${s.notifications} link=${link} />
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
        ${s.docker?.state ? html`
          <${Card} title="Containers" value="N/A"
            to=${(s.docker.state === 'not_installed' && link('packages', '/recommended')) || link('docker', '/')}
            sub=${s.docker.state === 'not_installed' ? 'Docker is not installed' : 'Docker is not running'} />` : s.docker ? html`
          <${Card} title="Containers" to=${link('docker', '/containers')} error=${s.docker.error}
            value=${s.docker.containers} sub=${`${s.docker.running} running`} />
          <${Card} title="Apps" to=${link('docker', '/apps')}
            value=${s.docker.apps} sub=${`${s.docker.apps_running} running`} />` : null}
        ${s.certificates ? html`
          <${Card} title="Certificates" to=${link('acme', '/certificates')} error=${s.certificates.error}
            value=${s.certificates.active}
            sub=${s.certificates.expiring ? `${s.certificates.expiring} expiring within 30 days` : s.certificates.pending ? `${s.certificates.pending} being issued` : 'Valid certificates'} />` : null}
        ${s.gateway ? html`
          <${Card} title="Gateway sites" to=${link('gateway', '/sites')}
            value=${s.gateway.sites} sub=${s.gateway.enabled ? `${s.gateway.requests} requests` : 'Off: serving nothing'} />` : null}
        ${s.dns ? html`
          <${Card} title="DNS queries" to=${link('dns', '/')}
            value=${s.dns.queries} sub=${s.dns.enabled ? `${s.dns.blocked} blocked · ${s.dns.zones} zones` : 'Off: not answering queries'} />` : null}
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
      ${s.gateway?.enabled ? html`<div class="section"><h2>Gateway listeners</h2>
        <${Table}>
          <thead><tr><th>Name</th><th>Address</th><th>TLS</th><th>Status</th></tr></thead>
          <tbody>${s.gateway.listeners.map(l => html`<tr key=${l.name}>
            <td>${l.name}</td><td class="mono">${l.address}</td><td>${l.tls ? html`<${Badge} variant="info">TLS<//>` : '—'}</td>
            <td>${l.running ? html`<${Status} state="running" />` : html`<${Status} state="failed" label=${l.error || 'Not running'} />`}</td>
          </tr>`)}</tbody>
        <//>
      </div>` : null}
      ${s.dns?.enabled ? html`<div class="section"><h2>DNS listeners</h2>
        <${Table}>
          <thead><tr><th>Protocol</th><th>Address</th><th>TLS</th><th>Status</th></tr></thead>
          <tbody>${s.dns.listeners.map(l => html`<tr key=${l.type + l.addr}>
            <td>${l.type.toUpperCase()}</td><td class="mono">${l.addr}</td><td>${l.tls ? html`<${Badge} variant="info">TLS<//>` : '—'}</td>
            <td>${l.error ? html`<${Status} state="failed" label=${l.error} />` : html`<${Status} state="running" />`}</td>
          </tr>`)}</tbody>
        <//>
      </div>` : null}`}`;
}
