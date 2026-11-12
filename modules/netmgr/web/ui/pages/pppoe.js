import { html } from '@lsongdev/panel/html.js';
import { Alert, Loading, Badge } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page, useApi } from '../network.js';

// What each missing piece is and how to get it. The module does not know the
// distribution, so it names the package where the name is the same everywhere
// and describes the rest.
const REQUIREMENTS = {
  pppd: {
    title: 'pppd',
    what: 'The PPP daemon that speaks PPPoE to the provider.',
    fix: html`Install the package <code>ppp</code>.`,
  },
  'NetworkManager PPP plugin': {
    title: 'NetworkManager PPP plugin',
    what: 'Lets NetworkManager start and watch PPP connections.',
    fix: html`Debian and Ubuntu include it in <code>network-manager</code>; Fedora and RHEL have a separate package, <code>NetworkManager-ppp</code>.`,
  },
};

const ALL = Object.keys(REQUIREMENTS);

// PPPoE is the dial-up connections page. For now it tells whether this
// machine can dial, and what to install when it cannot.
export default function PPPoE() {
  const { link } = usePanel();
  const info = useApi('/info');
  const i = info.data;
  const features = i?.features;
  const missing = features?.pppoe?.missing || [];
  const install = link?.('packages', '/', { tab: 'search', s: 'ppp' });

  return html`<${Page} title="PPPoE" subtitle="Dial-up connections to a provider">
    <${Alert} error=${info.error} />
    ${!i ? (info.error ? null : html`<${Loading} />`)
      : i.backend !== 'NetworkManager' ? html`<${Alert} variant="info">
          PPPoE connections are managed through NetworkManager, which is not what runs on this machine${i.backend ? ` (${i.backend})` : ''}.
        <//>`
      : html`
        ${features.pppoe.ready
          ? html`<${Alert} variant="success"><strong>This machine can dial.</strong> pppd and NetworkManager's PPP plugin are both installed.<//>`
          : html`<${Alert} variant="warning">
              <strong>This machine cannot dial yet.</strong> Not installed: ${missing.join(', ')}.
            <//>`}
        <section class="section">
          <h2 class="section-title">What PPPoE needs</h2>
          <div class="card">
            <ul class="checks" aria-label="Requirements">
              ${ALL.map(name => {
                const r = REQUIREMENTS[name];
                const absent = missing.includes(name);
                return html`<li key=${name}>
                  <${Badge} variant=${absent ? 'warning' : 'success'}>${absent ? 'Missing' : 'Installed'}<//>
                  <strong>${r.title}</strong>
                  <span class="muted small">${r.what}</span>
                  ${absent ? html`<div class="small check-fix">${r.fix}</div>` : null}
                </li>`;
              })}
            </ul>
          </div>
        </section>
        ${features.pppoe.ready ? null : html`<section class="section">
          <h2 class="section-title">Install it</h2>
          <div class="card">
            <ol class="steps">
              <li>${install
                ? html`<a href=${install}>Search Packages for <code>ppp</code></a> and install it.`
                : html`Install the missing package with your system's package manager.`}</li>
              <li>No restart is needed. Come back here and check again.</li>
            </ol>
            <div class="form-actions">
              <button class="button button-primary" disabled=${info.loading} onClick=${info.reload}>Check again</button>
            </div>
          </div>
        </section>`}`}
  <//>`;
}
