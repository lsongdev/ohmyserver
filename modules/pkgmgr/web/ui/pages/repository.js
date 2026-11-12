import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { routeURL, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { PageHeader, Alert, Field, Loading, toast } from '@lsongdev/scripts/react/components/index.js';
import { useApi, useSystem, RunningJob, supportsRepositories } from '../packages.js';

export default function Repository({ params, query }) {
  const { href } = usePanel();
  const backPath = routeURL('/repositories', { q: query.q });
  const back = { href: href(backPath), label: 'Repositories' };
  const system = useSystem();
  const manager = system.data?.manager;
  const supported = supportsRepositories(manager);
  const path = params.id ? `/repositories/${encodeURIComponent(params.id)}` : null;
  const file = useApi(supported ? path : null);
  const error = (!system.data && system.error) || (!file.data && file.error);
  const title = path ? 'Edit source' : 'Add source';

  return html`<${PageHeader} title=${title} subtitle=${file.data?.path} back=${back} />
    ${error ? html`<${Alert} error=${error} />`
      : !system.data || (supported && path && !file.data) ? html`<${Loading} />`
      : !supported ? html`<${Alert} variant="info">Repository file management is available on APT and DNF.<//>`
      : html`
        <${RunningJob} job=${system.data.running} />
        <${RepositoryForm} file=${file.data} manager=${manager} path=${path} backPath=${backPath}
          jobRunning=${Boolean(system.data.running)} />
      `}
  `;
}

function RepositoryForm({ file, manager, path, backPath, jobRunning }) {
  const { api, navigate, href } = usePanel();
  const original = file?.content || '';
  const [name, setName] = useState('');
  const [content, setContent] = useState(original);
  const save = useAction();
  const dirty = Boolean(name || content !== original);
  const release = useUnsavedChanges(dirty, { pending: save.busy });
  const blocked = jobRunning || save.busy;
  const apt = manager === 'apt';
  const suffix = apt ? '(?:list|sources)' : 'repo';
  const hint = apt
    ? 'Use Enabled: no in a .sources stanza, or comment out the deb line in a .list file.'
    : 'Use enabled=0 in a repository section to disable it.';
  const done = message => {
    release();
    toast(message, 'success');
    navigate(backPath);
  };

  const submit = event => {
    event.preventDefault();
    if (blocked || !dirty) return;
    if (file && !window.confirm(`Replace the repository configuration in ${file.path}?`)) return;
    save.run(async () => {
      if (file) await api.put(path, { content, revision: file.revision });
      else await api.post('/repositories', { id: name, content });
      done(file ? 'Source saved' : 'Source added');
    });
  };

  const remove = () => {
    if (blocked || !window.confirm(`Delete ${file.path} and remove every source it defines?`)) return;
    save.run(async () => {
      await api.del(path, { query: { revision: file.revision } });
      done('Source deleted');
    });
  };

  return html`<form class="form repository-form" onSubmit=${submit}>
    ${file ? null : html`<${Field} label="Filename" required hint=${apt ? 'Ends in .sources or .list' : 'Ends in .repo'}>
      <input class="input mono" required maxlength="120" pattern=${`[A-Za-z0-9_][A-Za-z0-9_.\\-]*\\.${suffix}`}
        placeholder=${apt ? 'vendor.sources' : 'vendor.repo'} value=${name} disabled=${save.busy}
        onInput=${event => setName(event.currentTarget.value)} />
    <//>`}
    <${Field} label="Source configuration" hint=${hint}>
      <textarea class="input mono" rows="13" spellcheck="false" autocapitalize="off" autocomplete="off"
        value=${content} disabled=${save.busy} onInput=${event => setContent(event.currentTarget.value)}></textarea>
    <//>
    <${Alert} error=${save.error} />
    <div class="form-actions">
      <button class="button button-primary" disabled=${blocked || !dirty}>${save.busy ? 'Working…' : 'Save source'}</button>
      <a class="button" href=${href(backPath)}>Cancel</a>
      ${file?.removable ? html`<button class="button button-danger" type="button" disabled=${blocked} onClick=${remove}>Delete source</button>` : null}
    </div>
  </form>`;
}
