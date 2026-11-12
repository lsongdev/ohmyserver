import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { Alert, Loading, Empty, Table, Badge, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page, useApi, useChanged } from '../network.js';

// What each missing piece is and how to get it. The module does not know the
// distribution, so it names the package where the name is the same everywhere
// and describes the rest.
const REQUIREMENTS = {
  pppd: { what: 'The PPP daemon that speaks PPPoE to the provider.', fix: html`Install the package <code>ppp</code>.` },
  'NetworkManager PPP plugin': {
    what: 'Lets NetworkManager start and watch PPP connections.',
    fix: html`Debian and Ubuntu include it in <code>network-manager</code>; Fedora and RHEL have a separate package, <code>NetworkManager-ppp</code>.`,
  },
};

// PPPoE: dial-up connections to a provider, and whether this machine can dial.
// A connection has a password, so making or changing one is a page of its own
// (pppoe-form.js); this lists them.
export default function PPPoE() {
  const { href, link } = usePanel();
  const info = useApi('/info');
  const supported = info.data?.backend === 'NetworkManager';
  const pppoe = useApi(supported ? '/pppoe' : null, 5000);
  useChanged(pppoe.reload);
  const p = pppoe.data;
  const actions = p?.ready ? html`<a class="button button-primary" href=${href('/pppoe/new')}>New connection</a>` : null;
  const install = link?.('packages', '/', { tab: 'search', s: 'ppp' });
  return html`<${Page} title="PPPoE" subtitle="Dial-up connections to a provider" actions=${actions}>
    <${Alert} error=${info.error || pppoe.error} />
    ${!info.data ? (info.error ? null : html`<${Loading} />`)
      : !supported ? html`<${Alert} variant="info">PPPoE connections are managed through NetworkManager, which is not what runs on this machine${info.data.backend ? ` (${info.data.backend})` : ''}.<//>`
      : !p ? (pppoe.error ? null : html`<${Loading} />`)
      : !p.ready ? html`<${NotReady} missing=${p.missing || []} install=${install} reload=${pppoe.reload} />`
      : p.connections.length === 0 ? html`<${Empty}>No connections. Make one with the user name and password your provider gave you.<//>`
      : html`<${Connections} connections=${p.connections} reload=${pppoe.reload} />`}
  <//>`;
}

function NotReady({ missing, install, reload }) {
  return html`
    <${Alert} variant="warning"><strong>This machine cannot dial yet.</strong> Not installed: ${missing.join(', ')}.<//>
    <section class="section">
      <h2 class="section-title">What PPPoE needs</h2>
      <div class="card"><ul class="checks" aria-label="Requirements">
        ${Object.entries(REQUIREMENTS).map(([name, r]) => html`<li key=${name}>
          <${Badge} variant=${missing.includes(name) ? 'warning' : 'success'}>${missing.includes(name) ? 'Missing' : 'Installed'}<//>
          <strong>${name}</strong> <span class="muted small">${r.what}</span>
          ${missing.includes(name) ? html`<div class="small check-fix">${r.fix}</div>` : null}
        </li>`)}
      </ul></div>
    </section>
    <section class="section">
      <h2 class="section-title">Install it</h2>
      <div class="card">
        <ol class="steps">
          <li>${install ? html`<a href=${install}>Search Packages for <code>ppp</code></a> and install it.` : html`Install the missing package with your system's package manager.`}</li>
          <li>No restart is needed. Come back here and check again.</li>
        </ol>
        <div class="form-actions"><button class="button button-primary" onClick=${reload}>Check again</button></div>
      </div>
    </section>`;
}

function Connections({ connections, reload }) {
  const { api, href } = usePanel();
  const action = useAction();
  const path = c => `/pppoe/${encodeURIComponent(c.name)}`;
  // Dialing is on trial (see TrialBanner): the banner appears once it is up.
  const dial = c => action.run(async () => {
    try { await api.post(`${path(c)}/dial`); } catch (err) { if (err.code !== 'network') throw err; }
    toast(`Dialing ${c.name}. Keep the connection to make it permanent.`, 'info');
    reload();
  });
  const hangup = c => {
    if (!window.confirm(`Hang up ${c.name}? Everything using it loses its connection until it is dialed again.`)) return;
    action.run(async () => { await api.post(`${path(c)}/hangup`); toast(`${c.name} hung up`, 'success'); reload(); });
  };
  // A forgotten connection cannot be brought back: its password goes with it.
  const forget = c => {
    if (!window.confirm(`Forget ${c.name}? Its user name and password are deleted.`)) return;
    action.run(async () => { await api.del(path(c)); toast(`${c.name} forgotten`, 'success'); reload(); });
  };
  return html`<${Alert} error=${action.error} />
    <${Table}>
      <thead><tr><th>Connection</th><th>Dials through</th><th>Status</th><th></th></tr></thead>
      <tbody>${connections.map(c => html`<tr key=${c.uuid}>
        <td><span class="cell-main">${c.name}</span><div class="cell-sub">${c.username}${c.managed ? '' : ' · made another way'}</div></td>
        <td class="mono">${c.parent || '—'}</td>
        <td>${c.active ? html`<${Badge} variant="success">Connected<//> <span class="muted small mono">${c.device}</span>` : html`<span class="muted">Not connected</span>`}</td>
        <td><span class="tags">
          ${c.active
            ? html`<button class="button button-small" disabled=${action.busy || c.protected} title=${c.protected ? 'It carries this machine\'s default route or this connection' : null} onClick=${() => hangup(c)}>Hang up</button>`
            : html`<button class="button button-small button-primary" disabled=${action.busy} onClick=${() => dial(c)}>Dial</button>`}
          ${c.managed ? html`<a class="button button-small" href=${href(path(c))}>Edit</a>
            <button class="button button-small" disabled=${action.busy || c.protected} onClick=${() => forget(c)}>Forget</button>` : null}
        </span></td>
      </tr>`)}</tbody>
    <//>`;
}
