import { html, useState } from '../lib/html.js';
import { Page, Notice, Loading, Status, Field } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, useSystem, useStartJob, RunningJob, Unsupported } from '../packages.js';

export default function Recommended() {
  const system = useSystem();
  const s = system.data;
  if (!s) return html`<${Page} title="Recommended">${system.error ? html`<${Notice} error=${system.error} />` : html`<${Loading} />`}<//>`;
  return html`<${Page} title="Recommended" subtitle="Software that other parts of the panel build on, installed the recommended way.">
    ${s.manager ? html`<${RunningJob} job=${s.running} /><${Docker} system=${s} />` : html`<${Unsupported} system=${s} />`}
  <//>`;
}

function Docker({ system }) {
  const { link } = usePanel();
  const status = useApi('/docker', system.running ? 3000 : 0);
  const start = useStartJob();
  const d = status.data;
  // Hosts in mainland China (Alibaba Cloud Linux there) reach the mirror
  // more reliably than download.docker.com.
  const [source, setSource] = useState(system.os.id === 'alinux' ? 'aliyun' : 'docker');
  const install = () => window.confirm('Install Docker Engine and start it?') && start({ action: 'docker', source });
  const open = link?.('docker', '/');
  return html`<div class="card recipe">
    <div class="recipe-head">
      <h2>Docker</h2>
      ${d ? (d.installed
        ? html`<${Status} state=${d.running ? 'running' : 'stopped'} label=${d.running ? 'Running' : 'Installed, not running'} />`
        : html`<${Status} state="missing" label="Not installed" />`) : null}
      ${d?.version ? html`<span class="muted small">Version ${d.version}</span>` : null}
    </div>
    <p class="muted">Docker Engine with the Buildx and Compose plugins, from Docker's own package repository, as${' '}
      <a href="https://docs.docker.com/engine/install/" target="_blank" rel="noopener">its documentation</a> recommends.
      Containers, apps and CI builds need it.</p>
    <${Notice} error=${status.error} />
    ${!d ? (status.error ? null : html`<${Loading} />`) : d.installed ? html`
      <div class="button-actions">
        ${open ? html`<a class="button" href=${open}>Open Docker</a>` : null}
        ${!d.running ? html`<span class="muted small">${system.manager === 'brew'
          ? 'Start Docker Desktop or OrbStack.' : html`Start it with <code>systemctl start docker</code>.`}</span>` : null}
      </div>` : d.unsupported ? html`<${Notice} tone="warn">${d.unsupported}.<//>` : html`
      ${system.manager === 'pacman' ? html`<p class="muted small">Arch Linux packages Docker in its own repositories.</p>` : html`<${Field} label="Download from">
        <select class="input" value=${source} onChange=${e => setSource(e.currentTarget.value)}>
          ${d.sources.map(s => html`<option key=${s.id} value=${s.id}>${s.title}</option>`)}
        </select>
      <//>`}
      <div class="button-actions">
        <button class="button button-primary" disabled=${Boolean(system.running)} onClick=${install}>Install Docker</button>
      </div>`}
  </div>`;
}
