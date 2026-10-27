import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Field, Modal, toast, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, BuildStatus, active, datetime, short, duration } from '../ci.js';

export default function Repos() {
  const { api, href, navigate } = usePanel();
  const repos = useApi('/repos', list => (list?.some(r => r.last_build && active(r.last_build.status)) ? 2000 : 10000));
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const run = r => action.run(async () => {
    const b = await api.post(`/repos/${r.id}/builds`);
    navigate(`/builds/${b.id}`);
  });
  return html`<${Page} title="Repositories" subtitle="Build and test in containers, as each repository's .ci.yml says, on push or on demand."
    actions=${html`<button class="button button-primary" onClick=${() => setAdding(true)}>Add repository</button>`}>
    <${Notice} error=${repos.error || action.error} />
    ${repos.data === undefined ? (repos.error ? null : html`<${Loading} />`) : !repos.data.length ? html`<${Empty}>No repositories yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Repository</th><th>Last build</th><th>Commit</th><th class="num">Duration</th><th class="actions"></th></tr></thead>
        <tbody>${repos.data.map(r => {
          const b = r.last_build;
          return html`<tr key=${r.id}>
            <td><a class="cell-main" href=${href(`/repos/${r.id}`)}>${r.name}</a><div class="cell-sub mono">${r.url}</div></td>
            <td>${b ? html`<a href=${href(`/builds/${b.id}`)}><${BuildStatus} status=${b.status} title=${b.error} /></a>
              <div class="cell-sub">#${b.number} · ${b.branch} · ${datetime(b.created_at)}</div>` : html`<span class="muted">No builds yet</span>`}</td>
            <td>${b?.commit ? html`<span class="commit">${short(b.commit)}</span><div class="cell-sub message" title=${b.message}>${b.message}</div>` : '—'}</td>
            <td class="num">${b ? duration(b.started_at, b.finished_at) : '—'}</td>
            <td class="actions"><span class="button-row">
              <button class="button button-small button-primary" disabled=${action.busy} onClick=${() => run(r)}>Run</button>
              <a class="button button-small" href=${href(`/repos/${r.id}`)}>Details</a>
            </span></td>
          </tr>`;
        })}</tbody>
      <//>`}
    ${adding ? html`<${RepoForm} onClose=${() => setAdding(false)} onSaved=${r => { setAdding(false); navigate(`/repos/${r.id}`); }} />` : null}
  <//>`;
}

// RepoForm adds or edits a repository. Secrets are edited as rows; a
// stored secret shows no value, and leaving it empty keeps it.
export function RepoForm({ repo, onClose, onSaved }) {
  const { api } = usePanel();
  const isNew = !repo;
  const [r, setR] = useState({
    name: repo?.name || '', url: repo?.url || '', branch: repo?.branch || 'main',
    config_path: repo?.config_path || '.ci.yml', timeout: repo?.timeout || 60, token: '',
  });
  const [secrets, setSecrets] = useState(() => (repo?.secret_names || []).map(name => ({ name, value: '', stored: true })));
  const { busy, error, run } = useAction();
  const set = patch => setR(prev => ({ ...prev, ...patch }));
  const setSecret = (i, patch) => setSecrets(list => list.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const submit = e => {
    e.preventDefault();
    run(async () => {
      const body = { ...r, timeout: Number(r.timeout) || 60, secrets: Object.fromEntries(secrets.filter(s => s.name.trim()).map(s => [s.name.trim(), s.value])) };
      const saved = isNew ? await api.post('/repos', body) : await api.put(`/repos/${repo.id}`, body);
      toast(isNew ? 'Repository added' : 'Repository saved', 'success');
      onSaved(saved);
    });
  };
  return html`<${Modal} wide title=${isNew ? 'Add repository' : `Edit ${repo.name}`} onClose=${onClose} footer=${html`
    <span class="spacer"></span>
    <button class="button" type="button" onClick=${onClose}>Cancel</button>
    <button class="button button-primary" type="submit" form="repo-form" disabled=${busy}>${isNew ? 'Add' : 'Save'}</button>`}>
    <form id="repo-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <div class="form-grid">
        <${Field} label="Name" required>
          <input class="input" required value=${r.name} onInput=${e => set({ name: e.currentTarget.value })} placeholder="ohmyserver" autofocus />
        <//>
        <${Field} label="Default branch" hint="Built when you run it by hand">
          <input class="input mono" value=${r.branch} onInput=${e => set({ branch: e.currentTarget.value })} />
        <//>
      </div>
      <${Field} label="Git URL" required hint="An HTTPS URL; for a private repository, also give an access token">
        <input class="input mono" required value=${r.url} onInput=${e => set({ url: e.currentTarget.value })} placeholder="https://github.com/you/app.git" />
      <//>
      <div class="form-grid">
        <${Field} label="Access token" hint=${!isNew && repo.has_token ? 'Saved; leave empty to keep it' : 'A GitHub or Gitea personal access token; empty for public repositories'}>
          <input class="input" type="password" autocomplete="new-password" value=${r.token} onInput=${e => set({ token: e.currentTarget.value })} />
        <//>
        <${Field} label="Configuration file">
          <input class="input mono" value=${r.config_path} onInput=${e => set({ config_path: e.currentTarget.value })} />
        <//>
        <${Field} label="Timeout (minutes)">
          <input class="input" type="number" min="1" max="1440" value=${r.timeout} onInput=${e => set({ timeout: e.currentTarget.value })} />
        <//>
      </div>
      <fieldset class="field" style="border:0;padding:0;margin:0;display:grid;gap:8px">
        <span class="field-label">Secrets</span>
        <span class="field-hint">A step gets a secret only by listing its name under secrets; logs mask the values. Saved values are not shown; leave one empty to keep it.</span>
        ${secrets.map((s, i) => html`<div class="secret-row" key=${i}>
          <input class="input mono" placeholder="NAME" value=${s.name} disabled=${s.stored} onInput=${e => setSecret(i, { name: e.currentTarget.value })} />
          <input class="input" type="password" autocomplete="new-password" placeholder=${s.stored ? 'Saved' : 'Value'} value=${s.value} onInput=${e => setSecret(i, { value: e.currentTarget.value })} />
          <button type="button" class="button button-ghost icon-button" aria-label="Remove" onClick=${() => setSecrets(list => list.filter((_, j) => j !== i))}>✕</button>
        </div>`)}
        <div><button type="button" class="button button-small" onClick=${() => setSecrets(list => [...list, { name: '', value: '' }])}>Add secret</button></div>
      </fieldset>
    </form>
  <//>`;
}
