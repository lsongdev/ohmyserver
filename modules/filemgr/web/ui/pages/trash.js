import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { bytes, datetime } from '../lib/format.js';
import { useApi, ConfirmDelete, count } from '../files.js';

export default function Trash() {
  const { api, navigate } = usePanel();
  const trash = useApi('/trash');
  const action = useAction();
  const [purging, setPurging] = useState(null);
  const items = trash.data || [];
  const restore = item => action.run(async () => {
    await api.post('trash/restore', { paths: [item.id] });
    toast(`Restored ${item.original}`, 'success');
    trash.reload();
  });
  const folder = p => p.slice(0, p.lastIndexOf('/')) || '/';
  return html`<${Page} title="Trash" subtitle="Deleted files wait here, on the disk they came from, until the trash is emptied."
    actions=${items.length ? html`<button class="button button-danger" onClick=${() => setPurging(items)}>Empty trash</button>` : null}>
    <${Notice} error=${trash.error || action.error} />
    ${!trash.data ? (trash.error ? null : html`<${Loading} />`) : items.length === 0 ? html`<${Empty}>The trash is empty.<//>` : html`
      <${Table}>
        <thead><tr><th>Name</th><th>Deleted from</th><th class="num">Size</th><th>Deleted</th><th class="actions"></th></tr></thead>
        <tbody>${items.map(item => html`<tr key=${item.id}>
          <td class="cell-main">${item.name}</td>
          <td><button class="link-button mono small" type="button" onClick=${() => navigate(`/?path=${encodeURIComponent(folder(item.original))}`)}>${folder(item.original)}</button></td>
          <td class="num">${bytes(item.bytes)}<div class="cell-sub">${count(item.files, 'file')}</div></td>
          <td class="nowrap muted">${datetime(item.deleted_at)}</td>
          <td class="actions"><span class="button-row">
            <button class="button button-small" disabled=${action.busy} onClick=${() => restore(item)}>Restore</button>
            <button class="button button-small button-danger" onClick=${() => setPurging([item])}>Delete for good</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
    ${purging ? html`<${ConfirmDelete} purge entries=${purging} onClose=${() => setPurging(null)}
      onDone=${label => { toast(`Deleted ${label} for good`, 'success'); setPurging(null); trash.reload(); }} />` : null}
  <//>`;
}
