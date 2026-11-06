import { html, useState } from '@lsongdev/panel/html.js';
import { PageHeader, Alert, Loading, Empty, Badge, Tabs, Table } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { bytes } from '@lsongdev/panel/format.js';
import { useApi, useSystem, useStartJob, RunningJob, Unsupported, PackageInfo } from '../packages.js';

// The installed list can hold thousands of packages: show the first ones
// and let the filter narrow it down.
const SHOW = 300;

export default function Packages() {
  const { query, setQuery } = usePanel();
  const system = useSystem();
  const s = system.data;
  const tab = query.tab || 'installed';
  const info = query.info || '';
  const setInfo = name => setQuery({ info: name || '' });
  const busy = Boolean(s?.running);

  if (!s) return html`<${PageHeader} title="Packages" />${system.error ? html`<${Alert} error=${system.error} />` : html`<${Loading} />`}`;
  if (!s.manager) return html`<${PageHeader} title="Packages" /><${Unsupported} system=${s} />`;
  const props = { busy, onInfo: setInfo };
  return html`<${PageHeader} title="Packages" subtitle=${`${s.os.name || s.os.id} · ${s.manager}`} />
    <${RunningJob} job=${s.running} />
    <${Tabs} active=${tab} onChange=${id => setQuery({ tab: id === 'installed' ? '' : id })}
      tabs=${[['installed', 'Installed'], ['updates', 'Updates'], ['search', 'Search']]} />
    ${info ? html`<${PackageInfo} key=${info} name=${info} onClose=${() => setInfo(null)} />` : null}
    ${tab === 'updates' ? html`<${Updates} ...${props} />`
      : tab === 'search' ? html`<${Search} ...${props} />`
      : html`<${Installed} ...${props} />`}`;
}

function Name({ pkg, onInfo }) {
  return html`<td>
    <button class="link-button cell-main" type="button" onClick=${() => onInfo(pkg.name)}>${pkg.name}</button>
    ${pkg.summary ? html`<div class="cell-sub truncate" style="max-width:480px" title=${pkg.summary}>${pkg.summary}</div>` : null}
  </td>`;
}

function Installed({ busy, onInfo }) {
  const { query, setQuery } = usePanel();
  const list = useApi('/installed');
  const start = useStartJob();
  const filter = (query.q || '').trim().toLowerCase();
  const all = list.data || [];
  const matches = filter ? all.filter(p => p.name.toLowerCase().includes(filter) || p.summary?.toLowerCase().includes(filter)) : all;
  const remove = p => window.confirm(`Remove ${p.name}? Packages that depend on it are removed too.`) &&
    start({ action: 'remove', packages: [p.name] });
  return html`
    <div class="toolbar">
      <input class="input" type="search" placeholder="Filter by name or summary" aria-label="Filter"
        value=${query.q || ''} onInput=${e => setQuery({ q: e.currentTarget.value })} />
      ${list.data ? html`<span class="muted small">${filter ? `${matches.length} of ${all.length}` : `${all.length} packages`}</span>` : null}
    </div>
    <${Alert} error=${list.error} />
    ${!list.data ? (list.error ? null : html`<${Loading} />`) : matches.length === 0 ? html`<${Empty}>No installed package matches.<//>` : html`
      <${Table}>
        <thead><tr><th>Name</th><th>Version</th><th>Arch</th><th class="num">Size</th><th class="actions"></th></tr></thead>
        <tbody>${matches.slice(0, SHOW).map(p => html`<tr key=${p.name + p.arch}>
          <${Name} pkg=${p} onInfo=${onInfo} />
          <td class="version">${p.version}</td>
          <td class="muted">${p.arch}</td>
          <td class="num muted">${p.size ? bytes(p.size) : '—'}</td>
          <td class="actions"><button class="button button-small button-danger" disabled=${busy} onClick=${() => remove(p)}>Remove</button></td>
        </tr>`)}</tbody>
      <//>
      ${matches.length > SHOW ? html`<p class="muted small">Showing ${SHOW} of ${matches.length}. Filter to find others.</p>` : null}`}`;
}

function Updates({ busy, onInfo }) {
  const list = useApi('/updates');
  const system = useSystem();
  const start = useStartJob();
  const updates = list.data || [];
  // Arch upgrades the whole system at once (pacman -Syu), never some
  // packages only.
  const whole = system.data?.manager === 'pacman';
  const all = () => window.confirm(whole
    ? `Upgrade the whole system (pacman -Syu), ${updates.length} packages including the kernel if listed? Packages tied to the kernel, such as ZFS modules from other repositories, can block or break the upgrade.`
    : `Upgrade all ${updates.length} packages?`) && start({ action: 'upgrade', packages: [] });
  return html`
    <div class="toolbar">
      <button class="button" disabled=${busy} onClick=${() => start({ action: 'refresh' })}
        title="Download the latest package lists from the repositories">Check for updates</button>
      ${updates.length ? html`<button class="button button-primary" disabled=${busy} onClick=${all}>Upgrade all (${updates.length})</button>` : null}
    </div>
    <${Alert} error=${list.error} />
    ${whole && updates.length ? html`<p class="muted small">Arch Linux upgrades the whole system at once; single packages cannot be upgraded.</p>` : null}
    ${!list.data ? (list.error ? null : html`<${Loading}>Reading the package lists…<//>`) : updates.length === 0
      ? html`<${Empty}>Everything is up to date, as of the last package list refresh.<//>` : html`
      <${Table}>
        <thead><tr><th>Name</th><th>Installed</th><th>Available</th><th>Repository</th><th class="actions"></th></tr></thead>
        <tbody>${updates.map(u => html`<tr key=${u.name + u.arch}>
          <${Name} pkg=${u} onInfo=${onInfo} />
          <td class="version muted">${u.current || '—'}</td>
          <td class="version">${u.version}</td>
          <td class="muted">${u.repo}</td>
          <td class="actions">${whole ? null : html`<button class="button button-small" disabled=${busy}
            onClick=${() => start({ action: 'upgrade', packages: [u.arch ? `${u.name}.${u.arch}` : u.name] })}>Upgrade</button>`}</td>
        </tr>`)}</tbody>
      <//>`}`;
}

function Search({ busy, onInfo }) {
  const { query, setQuery } = usePanel();
  const [text, setText] = useState(query.s || '');
  const term = query.s || '';
  const results = useApi(term ? `/search?q=${encodeURIComponent(term)}` : null);
  const start = useStartJob();
  const submit = e => { e.preventDefault(); setQuery({ s: text.trim() }); };
  return html`
    <form class="toolbar" onSubmit=${submit}>
      <input class="input" type="search" placeholder="Package name, such as nginx" aria-label="Package name"
        required minlength="2" value=${text} onInput=${e => setText(e.currentTarget.value)} />
      <button class="button button-primary">Search</button>
    </form>
    <${Alert} error=${results.error} />
    ${!term ? html`<${Empty}>Search the configured repositories by package name.<//>`
      : !results.data ? (results.error ? null : html`<${Loading}>Searching…<//>`)
      : results.data.length === 0 ? html`<${Empty}>No package name contains “${term}”.<//>` : html`
      <${Table}>
        <thead><tr><th>Name</th><th>Version</th><th>Repository</th><th class="actions"></th></tr></thead>
        <tbody>${results.data.map(p => html`<tr key=${p.name + p.arch}>
          <${Name} pkg=${p} onInfo=${onInfo} />
          <td class="version">${p.version || '—'}</td>
          <td class="muted">${p.repo || '—'}</td>
          <td class="actions">${p.installed ? html`<${Badge} variant="success">Installed<//>`
            : html`<button class="button button-small button-primary" disabled=${busy}
                onClick=${() => start({ action: 'install', packages: [p.name] })}>Install</button>`}</td>
        </tr>`)}</tbody>
      <//>
      ${results.data.length >= 200 ? html`<p class="muted small">Showing the best 200 matches. Search for a longer name to narrow them.</p>` : null}`}`;
}
