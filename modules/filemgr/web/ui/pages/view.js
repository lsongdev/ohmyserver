import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { returnPath, routeURL, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { bytes } from '@lsongdev/panel/format.js';
import { useApi, parent, isImage } from '../files.js';

export default function FilePage() {
  const { query } = usePanel();
  return html`<${Workspace} key=${query.path} path=${query.path || ''} />`;
}

function Workspace({ path }) {
  const { href, query, client } = usePanel();
  const image = isImage(path);
  const file = useApi(path && !image ? `/text?path=${encodeURIComponent(path)}` : null);
  const back = returnPath(query, routeURL('/', { path: parent(path) }));
  const download = client.url('download', { path }).href;
  return html`<${PageHeader} title=${path.split('/').pop() || 'File'}
    back=${{ href: href(back), label: 'Files' }} subtitle=${html`<span class="mono">${path}</span>`}
    actions=${path ? html`<a class="button" href=${download} download>Download</a>` : null} />
    ${!path ? html`<${Alert} variant="danger">Choose a file to open.<//>`
      : image ? html`<img class="preview-image" src=${client.url('download', { path, inline: 1 }).href} alt=${path} />`
      : file.error ? html`<${Alert} error=${file.error} />`
      : !file.data ? html`<${Loading} />`
      : file.data.binary || file.data.too_large ? html`<${Alert} variant="info">
        ${file.data.too_large ? `This file is ${bytes(file.data.size)}, too large to open here.` : 'This file is not text.'} Download it instead.
      <//>` : html`<${Editor} path=${path} content=${file.data.text || ''} />`}`;
}

function Editor({ path, content }) {
  const { api } = usePanel();
  const [text, setText] = useState(content);
  const [baseline, setBaseline] = useState(content);
  const save = useAction();
  const dirty = text !== baseline;
  const saved = useUnsavedChanges(dirty, { pending: save.busy });
  const submit = event => {
    event.preventDefault();
    save.run(async () => {
      await api.put('text', { path, text });
      setBaseline(text);
      saved();
      toast('File saved', 'success');
    });
  };
  return html`<form class="form file-workspace" onSubmit=${submit}>
    <div class="button-actions">
      <button class="button button-primary" disabled=${!dirty || save.busy}>${save.busy ? 'Saving…' : 'Save'}</button>
      <span class="muted small" role="status">${dirty ? 'Unsaved changes' : 'Saved'}</span>
    </div>
    <${Alert} error=${save.error} />
    <textarea class="input editor" aria-label=${`Contents of ${path}`} spellcheck="false" autofocus
      disabled=${save.busy} value=${text} onInput=${event => setText(event.currentTarget.value)} />
  </form>`;
}
