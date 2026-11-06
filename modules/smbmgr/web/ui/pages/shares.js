import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Page, Notice, Loading, Empty, Table, Badge, toast } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useApi, Unsupported, ServiceStatus } from '../samba.js';

export default function Shares() {
  const { api, href, link } = usePanel();
  const info = useApi('/info');
  const list = useApi(info.data?.installed ? '/shares' : null);
  const reload = useAction();

  if (info.data && !info.data.installed) return html`<${Page} title="Shares"><${Unsupported} /><//>`;
  const i = info.data;
  const unit = i?.service?.unit;
  const unitLink = unit && link('services', `/services/${encodeURIComponent(unit)}`);
  const doReload = () => reload.run(async () => {
    await api.post('/reload');
    toast('smbd reloaded smb.conf', 'success');
  });
  const shares = list.data?.shares || [];

  return html`<${Page} title="Shares" subtitle=${i?.version ? `Samba ${i.version}` : null} actions=${html`<span class="page-actions">
      <button class="button" disabled=${reload.busy} onClick=${doReload} title="Ask smbd to read smb.conf again">Reload</button>
      <a class="button button-primary" href=${href('/shares/new')}>Add share</a>
    </span>`}>
    <${Notice} error=${info.error || list.error || reload.error} />
    ${i ? html`<div class="card server">
      <span>smbd <${ServiceStatus} service=${i.service} /></span>
      ${unitLink ? html`<a href=${unitLink}>${unit}</a>` : unit ? html`<code>${unit}</code>` : null}
      <span class="muted">Configuration <a href=${href('/global')}><code>${i.config}</code></a></span>
    </div>` : null}
    <section class="section">
      ${!list.data ? (list.error ? null : html`<${Loading} />`) : shares.length === 0 ? html`<${Empty}>No shares yet. <a href=${href('/shares/new')}>Add one</a> to make a folder available on the network.<//>` : html`
        <${Table}>
          <thead><tr><th>Share</th><th>Path</th><th>Access</th><th>Users</th></tr></thead>
          <tbody>${shares.map(s => html`<tr key=${s.name}>
            <td>
              <a class="cell-main" href=${href(`/shares/${encodeURIComponent(s.name)}`)}>${s.name}</a>
              ${s.comment ? html`<div class="cell-sub">${s.comment}</div>` : null}
            </td>
            <td><${SharePath} share=${s} /></td>
            <td><span class="tags">
              ${s.printable ? html`<${Badge} tone="blue">Printer<//>` : s.read_only ? html`<${Badge}>Read only<//>` : html`<${Badge} tone="green">Read & write<//>`}
              ${s.guest ? html`<${Badge} tone="amber">Guests<//>` : null}
              ${s.browseable ? null : html`<${Badge} title="Not listed when browsing the server">Hidden<//>`}
              ${s.available ? null : html`<${Badge} tone="red">Off<//>`}
            </span></td>
            <td class="cell-sub">${s.valid_users || (s.guest ? 'Anyone' : 'Any Samba user')}${s.write_list ? html`<div>Writers: ${s.write_list}</div>` : null}</td>
          </tr>`)}</tbody>
        <//>`}
    </section>
  <//>`;
}

function SharePath({ share }) {
  const { link } = usePanel();
  if (!share.path) return html`<span class="muted">${share.special ? 'Each user\'s home' : '—'}</span>`;
  const files = !share.path.includes('%') && link('files', '/', { path: share.path });
  return html`${files ? html`<a class="mono" href=${files}>${share.path}</a>` : html`<code>${share.path}</code>`}
    ${share.path_missing ? html` <${Badge} tone="red" title="This folder does not exist">Missing<//>` : null}`;
}
