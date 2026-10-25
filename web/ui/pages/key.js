import { html } from '../lib/html.js';
import { useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, CopyButton } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';

export default function Key() {
  const { api } = usePanel();
  const key = useResource(signal => api.get('/key', { signal }), []);
  const line = key.data?.public_key;
  return html`<${Page} title="面板公钥" subtitle="认证方式选「面板公钥」的主机，用这把密钥登录，不必保存密码。">
    <${Notice} error=${key.error} />
    ${!line ? (key.error ? null : html`<${Loading} />`) : html`
      <div class="card">
        <div class="card-head" style="display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:12px">
          <h2 style="margin:0">公钥</h2><${CopyButton} text=${line} />
        </div>
        <pre class="key-line">${line}</pre>
      </div>
      <div class="card section">
        <h2>添加到主机</h2>
        <p class="muted" style="margin-top:0">在目标主机上，以要登录的用户执行：</p>
        <pre class="key-line">${`mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${line}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`}</pre>
        <div style="margin-top:10px"><${CopyButton} text=${`mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${line}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`} label="复制命令" /></div>
      </div>`}
  <//>`;
}
