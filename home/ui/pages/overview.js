// The overview links into other modules with ctx.link, which returns null
// for modules that are not mounted, so this page adapts to the build.
import { html, useMemo } from '../../../../_shell/html.js';
import { useResource } from '../../../../_shell/hooks.js';
import { createClient } from '../../../../_shell/client.js';
import { Page, Notice, Loading, Status, Badge } from '../../../../_shell/ui.js';

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
  return html`<${Page} title="概览">
    <${Notice} error=${summary.error} />
    ${!s ? (summary.error ? null : html`<${Loading} />`) : html`
      <div class="grid">
        ${s.docker ? html`
          <${Card} title="容器" to=${link('docker', '/containers')} error=${s.docker.error}
            value=${s.docker.containers} sub=${`${s.docker.running} 运行中`} />
          <${Card} title="应用" to=${link('docker', '/apps')}
            value=${s.docker.apps} sub=${`${s.docker.apps_running} 运行中`} />` : null}
        ${s.certificates ? html`
          <${Card} title="证书" to=${link('acme', '/certificates')} error=${s.certificates.error}
            value=${s.certificates.active}
            sub=${s.certificates.expiring ? `${s.certificates.expiring} 张 30 天内到期` : s.certificates.pending ? `${s.certificates.pending} 张签发中` : '有效证书'} />` : null}
        ${s.gateway ? html`
          <${Card} title="网关站点" to=${link('yabber', '/sites')}
            value=${s.gateway.sites} sub=${`${s.gateway.requests} 次请求`} />` : null}
        ${s.dns ? html`
          <${Card} title="DNS 查询" to=${link('dns', '/')}
            value=${s.dns.queries} sub=${`${s.dns.blocked} 次拦截 · ${s.dns.zones} 个区域`} />` : null}
        ${s.forward ? html`
          <${Card} title="端口转发" to=${link('forward', '/')}
            value=${s.forward.running}
            sub=${s.forward.failed ? `${s.forward.failed} 条未能启动` : `${s.forward.rules} 条规则 · ${s.forward.active} 个连接`} />` : null}
        ${s.cron ? html`
          <${Card} title="定时任务" to=${link('cron', '/')}
            value=${s.cron.enabled}
            sub=${s.cron.failing ? `${s.cron.failing} 个最近一次失败` : s.cron.running ? `${s.cron.running} 个运行中` : `共 ${s.cron.jobs} 个任务`} />` : null}
        ${s.ssh ? html`
          <${Card} title="SSH 主机" to=${link('ssh', '/')}
            value=${s.ssh.hosts} sub=${`${s.ssh.sessions} 个终端在线`} />` : null}
      </div>
      ${s.gateway ? html`<div class="section"><h2>网关监听</h2>
        <div class="table-wrap"><table class="table">
          <thead><tr><th>名称</th><th>地址</th><th>TLS</th><th>状态</th></tr></thead>
          <tbody>${s.gateway.listeners.map(l => html`<tr key=${l.name}>
            <td>${l.name}</td><td class="mono">${l.address}</td><td>${l.tls ? html`<${Badge} tone="blue">TLS<//>` : '—'}</td>
            <td>${l.running ? html`<${Status} state="running" />` : html`<${Status} state="failed" label=${l.error || '未运行'} />`}</td>
          </tr>`)}</tbody>
        </table></div>
      </div>` : null}
      ${s.dns ? html`<div class="section"><h2>DNS 监听</h2>
        <div class="table-wrap"><table class="table">
          <thead><tr><th>协议</th><th>地址</th><th>TLS</th><th>状态</th></tr></thead>
          <tbody>${s.dns.listeners.map(l => html`<tr key=${l.type + l.addr}>
            <td>${l.type.toUpperCase()}</td><td class="mono">${l.addr}</td><td>${l.tls ? html`<${Badge} tone="blue">TLS<//>` : '—'}</td>
            <td>${l.error ? html`<${Status} state="failed" label=${l.error} />` : html`<${Status} state="running" />`}</td>
          </tr>`)}</tbody>
        </table></div>
      </div>` : null}`}
  <//>`;
}
