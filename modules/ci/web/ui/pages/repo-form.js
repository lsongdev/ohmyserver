import { html, useState } from '../lib/html.js';
import { useAction, useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { FormSection, returnPath, useUnsavedChanges } from '../lib/forms.js';

export default function RepositoryEditor({ params }) {
  const { api, href, query } = usePanel();
  const isNew = !params.id;
  const repo = useResource(signal => isNew ? Promise.resolve(null) : api.get(`/repos/${params.id}`, { signal }), [params.id]);
  const back = returnPath(query, isNew ? '/' : `/repos/${params.id}`);
  const title = isNew ? 'Add repository' : repo.data ? `Edit ${repo.data.name}` : 'Edit repository';
  return html`<${Page} title=${title} back=${{ href: href(back), label: back.startsWith('/repos/') ? 'Repository' : 'Repositories' }}>
    ${!isNew && !repo.data ? (repo.error ? html`<${Notice} error=${repo.error} />` : html`<${Loading} />`)
      : html`<${RepoForm} repo=${repo.data} back=${back} />`}
  <//>`;
}

// RepoForm adds or edits a repository. Secrets are edited as rows; a
// stored secret shows no value, and leaving it empty keeps it.
function RepoForm({ repo, back }) {
  const { api, href, navigate } = usePanel();
  const isNew = !repo;
  const [r, setR] = useState({
    name: repo?.name || '', url: repo?.url || '', branch: repo?.branch || 'main',
    config_path: repo?.config_path || '.ci.yml', timeout: String(repo?.timeout || 60), token: '',
  });
  const [secrets, setSecrets] = useState(() => (repo?.secret_names || []).map(name => ({ name, value: '', stored: true })));
  const [initial] = useState(() => JSON.stringify({ r, secrets }));
  const { busy, error, run } = useAction();
  const saved = useUnsavedChanges(JSON.stringify({ r, secrets }) !== initial, { pending: busy });
  const set = patch => setR(prev => ({ ...prev, ...patch }));
  const setSecret = (i, patch) => setSecrets(list => list.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const submit = e => {
    e.preventDefault();
    run(async () => {
      const body = { ...r, timeout: Number(r.timeout) || 60, secrets: Object.fromEntries(secrets.filter(s => s.name.trim()).map(s => [s.name.trim(), s.value])) };
      if (isNew) await api.post('/repos', body);
      else await api.put(`/repos/${repo.id}`, body);
      toast(isNew ? 'Repository added' : 'Repository saved', 'success');
      saved();
      navigate(back, { replace: true });
    });
  };
  return html`<form class="form settings-form" onSubmit=${submit}>
      <fieldset class="form-fields" disabled=${busy}>
      <${Notice} error=${error} />
      <${FormSection} title="Repository">
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
      <//>
      <${FormSection} title="Build settings">
        <div class="form-grid">
          <${Field} label="Configuration file">
            <input class="input mono" value=${r.config_path} onInput=${e => set({ config_path: e.currentTarget.value })} />
          <//>
          <${Field} label="Timeout (minutes)">
            <input class="input" type="number" min="1" max="1440" value=${r.timeout} onInput=${e => set({ timeout: e.currentTarget.value })} />
          <//>
        </div>
      <//>
      <${FormSection} title="Credentials and secrets">
        <${Field} label="Access token" hint=${!isNew && repo.has_token ? 'Saved; leave empty to keep it' : 'A GitHub or Gitea personal access token; empty for public repositories'}>
          <input class="input" type="password" autocomplete="new-password" value=${r.token} onInput=${e => set({ token: e.currentTarget.value })} />
        <//>
      <fieldset class="field" style="border:0;padding:0;margin:0;display:grid;gap:8px">
        <span class="field-label">Secrets</span>
        <span class="field-hint">A step gets a secret only by listing its name under secrets; logs mask the values. Saved values are not shown; leave one empty to keep it.</span>
        ${secrets.map((s, i) => html`<div class="secret-row" key=${i}>
          <input class="input mono" aria-label="Secret name" placeholder="NAME" value=${s.name} disabled=${s.stored} onInput=${e => setSecret(i, { name: e.currentTarget.value })} />
          <input class="input" aria-label=${`Value for ${s.name || 'new secret'}`} type="password" autocomplete="new-password" placeholder=${s.stored ? 'Saved' : 'Value'} value=${s.value} onInput=${e => setSecret(i, { value: e.currentTarget.value })} />
          <button type="button" class="button button-ghost icon-button" aria-label="Remove" onClick=${() => setSecrets(list => list.filter((_, j) => j !== i))}>✕</button>
        </div>`)}
        <div><button type="button" class="button button-small" onClick=${() => setSecrets(list => [...list, { name: '', value: '' }])}>Add secret</button></div>
      </fieldset>
      <//>
      </fieldset>
      <div class="form-actions">
        <button class="button button-primary" disabled=${busy}>${busy ? 'Saving…' : isNew ? 'Add repository' : 'Save changes'}</button>
        <a class="button" href=${href(back)}>Cancel</a>
      </div>
    </form>`;
}
