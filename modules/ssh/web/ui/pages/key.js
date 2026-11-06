import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, CopyButton } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';

export default function Key() {
  const { api } = usePanel();
  const key = useResource(signal => api.get('/key', { signal }), []);
  const line = key.data?.public_key;
  return html`<${PageHeader} title="Panel key" subtitle="Hosts set to authenticate with the panel key sign in with this key, so no password is stored." />
    <${Alert} error=${key.error} />
    ${!line ? (key.error ? null : html`<${Loading} />`) : html`
      <div class="card">
        <div class="card-head" style="display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:12px">
          <h2 style="margin:0">Public key</h2><${CopyButton} text=${line} />
        </div>
        <pre class="key-line">${line}</pre>
      </div>
      <div class="card section">
        <h2>Add it to a host</h2>
        <p class="muted" style="margin-top:0">On the host, run this as the user to sign in as:</p>
        <pre class="key-line">${`mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${line}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`}</pre>
        <div style="margin-top:10px"><${CopyButton} text=${`mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${line}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`} label="Copy command" /></div>
      </div>`}`;
}
