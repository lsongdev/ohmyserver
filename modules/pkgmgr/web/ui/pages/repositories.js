import { html } from '@lsongdev/panel/html.js';
import { PageHeader, Alert, Loading, Empty } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Resource } from '@lsongdev/panel/ui.js';
import { useApi, useSystem, useStartJob, RunningJob, supportsRepositories } from '../packages.js';

export default function Repositories() {
  const { href, query, setQuery } = usePanel();
  const system = useSystem();
  const manager = system.data?.manager;
  const supported = supportsRepositories(manager);
  const files = useApi(supported ? '/repositories' : null);
  const start = useStartJob();
  const filter = (query.q || '').toLowerCase().trim();
  const busy = Boolean(system.data?.running);
  const actions = supported ? html`
    <button class="button" disabled=${busy} onClick=${() => start({ action: 'refresh' })}>Refresh metadata</button>
    <a class="button button-primary" href=${href('/repositories/new', { q: query.q })}>Add source</a>
  ` : null;

  return html`<${PageHeader} title="Repositories" actions=${actions} />
    ${!system.data ? (system.error ? html`<${Alert} error=${system.error} />` : html`<${Loading} />`)
      : !supported ? html`<${Empty}>Repository file management is available on APT and DNF.<//>`
      : html`
        <${RunningJob} job=${system.data.running} />
        <div class="toolbar">
          <input class="input" type="search" aria-label="Filter repository files" placeholder="Filter files"
            value=${query.q || ''} onInput=${e => setQuery({ q: e.currentTarget.value })} />
        </div>
        <${Resource} resource=${files} empty="No repository files found.">
          ${items => {
            const matches = items.filter(file => file.path.toLowerCase().includes(filter));
            return matches.length ? html`<ul class="repository-files">${matches.map(file => html`
              <li class="card" key=${file.id}>
                <a href=${href(`/repositories/${encodeURIComponent(file.id)}`, { q: query.q })}>${file.path}</a>
              </li>`)}</ul>` : html`<${Empty}>No files match your filter.<//>`;
          }}
        <//>
      `}
  `;
}
