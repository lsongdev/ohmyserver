import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

function TextField({ value, label, name, onChange, type = 'text', required = false, min, step, placeholder }) {
  return html`<label class="form-field"><span>${label}</span><input class="input" name=${name} type=${type} value=${value ?? ''} required=${required} min=${min} step=${step} placeholder=${placeholder} onInput=${e => onChange(name, e.currentTarget.value)}/></label>`;
}
function TextArea({ value, label, name, onChange, rows = 4 }) {
  return html`<label class="form-field"><span>${label}</span><textarea class="input" name=${name} rows=${rows} value=${value ?? ''} onInput=${e => onChange(name, e.currentTarget.value)}></textarea></label>`;
}
function SelectField({ value, label, name, onChange, options, required = false }) {
  return html`<label class="form-field"><span>${label}</span><select class="input" name=${name} value=${value ?? ''} required=${required} onChange=${e => onChange(name, e.currentTarget.value)}>${options.map(([key, text]) => html`<option value=${key} key=${key}>${text}</option>`)}</select></label>`;
}
function CheckField({ value, label, name, onChange }) {
  return html`<label class="web-enabled"><input class="input" type="checkbox" name=${name} checked=${Boolean(value)} onChange=${e => onChange(name, e.currentTarget.checked)}/> ${label}</label>`;
}
function MultiField({ values = [], label, name, onChange, options }) {
  return html`<label class="form-field"><span>${label}</span><select class="input" multiple value=${values.map(String)} onChange=${e => onChange(name, Array.from(e.currentTarget.selectedOptions, option => Number(option.value)))}>${options.map(item => html`<option value=${item.id} key=${item.id}>${item.name}</option>`)}</select></label>`;
}
const get = (value, key, fallback = '') => value?.[key] ?? fallback;
function useDraft(value) {
  const [draft, setDraft] = useState(() => ({ ...value }));
  useEffect(() => setDraft({ ...value }), [value]);
  const change = (key, item) => setDraft(current => ({ ...current, [key]: item }));
  return [draft, change];
}

export function ListenerForm({ value = {}, policies = [], certificates = [], onSubmit, submitLabel = '保存', busy = false }) {
  const [draft, change] = useDraft(value);
  return html`<form class="web-config-form" onSubmit=${e => { e.preventDefault(); onSubmit(draft); }}>
    <${TextField} value=${get(draft,'name')} label="名称" name="name" required=${true} onChange=${change}/>
    <${TextField} value=${get(draft,'address',':8080')} label="监听地址" name="address" required=${true} placeholder=":80" onChange=${change}/>
    <${SelectField} value=${get(draft,'protocol','http')} label="协议" name="protocol" required=${true} options=${[['http','HTTP'],['https','HTTPS']]} onChange=${change}/>
    <${SelectField} value=${get(draft,'min_tls_version','TLS1.2')} label="最低 TLS 版本" name="min_tls_version" options=${[['TLS1.2','TLS 1.2'],['TLS1.3','TLS 1.3']]} onChange=${change}/>
    ${draft.protocol === 'https' ? html`<${SelectField} value=${get(draft,'certificate_id','')} label="ACME 默认证书" name="certificate_id" required=${true} options=${[['','选择有效证书'],...certificates.map(c=>[String(c.id),c.domains])]} onChange=${change}/>` : null}
    <${TextField} value=${get(draft,'trusted_proxies')} label="可信代理 CIDR（每行一个）" name="trusted_proxies" onChange=${change}/>
    <${MultiField} values=${draft.policy_ids || []} label="Policies" name="policy_ids" options=${policies} onChange=${change}/>
    <${CheckField} value=${get(draft,'enabled',true)} label="启用" name="enabled" onChange=${change}/>
    <button class="button button-primary" type="submit" disabled=${busy}>${submitLabel}</button>
  </form>`;
}

export function HostForm({ value = {}, listeners = [], policies = [], certificates = [], onSubmit, submitLabel = '保存', busy = false }) {
  const [draft, change] = useDraft(value);
  return html`<form class="web-config-form" onSubmit=${e => { e.preventDefault(); onSubmit(draft); }}>
    <${TextField} value=${get(draft,'name')} label="名称" name="name" required=${true} onChange=${change}/>
    <${TextField} value=${get(draft,'hostnames')} label="域名（逗号分隔）" name="hostnames" required=${true} placeholder="example.com,www.example.com" onChange=${change}/>
    <${SelectField} value=${get(draft,'certificate_id','')} label="ACME 证书" name="certificate_id" options=${[['','由 HTTPS listener 提供'],...certificates.map(c=>[String(c.id),c.domains])]} onChange=${change}/>
    <${MultiField} values=${draft.listener_ids || []} label="Listeners" name="listener_ids" options=${listeners} onChange=${change}/>
    <${MultiField} values=${draft.policy_ids || []} label="Policies" name="policy_ids" options=${policies} onChange=${change}/>
    <${CheckField} value=${get(draft,'enabled',true)} label="启用" name="enabled" onChange=${change}/>
    <${CheckField} value=${get(draft,'force_https')} label="强制 HTTPS" name="force_https" onChange=${change}/>
    <button class="button button-primary" type="submit" disabled=${busy}>${submitLabel}</button>
  </form>`;
}

export function PolicyForm({ value = {}, onSubmit, submitLabel = '保存', busy = false }) {
  const [draft, change] = useDraft(value);
  return html`<form class="web-config-form" onSubmit=${e => { e.preventDefault(); onSubmit(draft); }}>
    <${TextField} value=${get(draft,'name')} label="名称" name="name" required=${true} onChange=${change}/>
    <${TextField} value=${get(draft,'max_body_bytes',0)} label="最大请求体（bytes）" name="max_body_bytes" type="number" min="0" onChange=${change}/>
    <${TextField} value=${get(draft,'request_timeout_seconds',0)} label="请求超时（秒）" name="request_timeout_seconds" type="number" min="0" onChange=${change}/>
    <${TextField} value=${get(draft,'rate_requests_per_second',0)} label="速率（请求/秒，0 不限制）" name="rate_requests_per_second" type="number" min="0" step="0.1" onChange=${change}/>
    <${TextField} value=${get(draft,'rate_burst',0)} label="Rate burst" name="rate_burst" type="number" min="0" onChange=${change}/>
    <${SelectField} value=${get(draft,'rate_key','client_ip')} label="限流 key" name="rate_key" options=${[['client_ip','Client IP'],['global','Global']]} onChange=${change}/>
    <${TextField} value=${get(draft,'rate_max_keys',10000)} label="最大限流 key 数" name="rate_max_keys" type="number" min="1" onChange=${change}/>
    <${TextField} value=${get(draft,'rate_idle_timeout_seconds',600)} label="限流状态空闲回收（秒）" name="rate_idle_timeout_seconds" type="number" min="1" onChange=${change}/>
    <${TextField} value=${get(draft,'concurrency_max',0)} label="并发上限（0 不限制）" name="concurrency_max" type="number" min="0" onChange=${change}/>
    <button class="button button-primary" type="submit" disabled=${busy}>${submitLabel}</button>
  </form>`;
}

export function RouteForm({ value = {}, policies = [], onSubmit, submitLabel = '保存', busy = false }) {
  const [draft, change] = useDraft(value);
  const handler = get(draft,'handler_type','reverse_proxy');
  return html`<form class="web-config-form" onSubmit=${e => { e.preventDefault(); onSubmit(draft); }}>
    <${TextField} value=${get(draft,'name')} label="Route 名称" name="name" required=${true} onChange=${change}/>
    <${TextField} value=${get(draft,'method')} label="HTTP 方法（空表示全部）" name="method" placeholder="GET" onChange=${change}/>
    <${TextField} value=${get(draft,'path','/')} label="匹配路径" name="path" required=${true} onChange=${change}/>
    <${SelectField} value=${handler} label="处理器" name="handler_type" options=${[['reverse_proxy','反向代理'],['file_server','静态文件'],['static_response','固定响应'],['redirect','重定向'],['worker','Worker 脚本']]} onChange=${change}/>
    ${handler === 'reverse_proxy' ? html`<${TextField} value=${get(draft,'upstream_url')} label="上游 URL" name="upstream_url" required=${true} placeholder="http://127.0.0.1:3000" onChange=${change}/>` : null}
    ${handler === 'file_server' ? html`<${TextField} value=${get(draft,'root')} label="静态文件根目录" name="root" required=${true} onChange=${change}/><${TextField} value=${get(draft,'strip_prefix')} label="移除路径前缀" name="strip_prefix" onChange=${change}/>` : null}
    ${handler === 'static_response' ? html`<${TextField} value=${get(draft,'response_status',200)} label="响应状态码" name="response_status" type="number" min="100" max="599" onChange=${change}/><${TextArea} value=${get(draft,'response_body')} label="响应内容" name="response_body" onChange=${change}/>` : null}
    ${handler === 'redirect' ? html`<${TextField} value=${get(draft,'redirect_location')} label="跳转地址" name="redirect_location" required=${true} onChange=${change}/><${TextField} value=${get(draft,'redirect_status',302)} label="跳转状态码" name="redirect_status" type="number" min="300" max="399" onChange=${change}/>` : null}
    ${handler === 'worker' ? html`<${TextArea} value=${get(draft,'worker_script')} label="Worker JavaScript" name="worker_script" rows="12" onChange=${change}/>` : null}
    <${TextField} value=${get(draft,'request_timeout_seconds',0)} label="请求超时（秒，0 使用 Policy）" name="request_timeout_seconds" type="number" min="0" onChange=${change}/>
    <${TextField} value=${get(draft,'response_header_timeout_seconds',0)} label="响应头超时（秒，0 使用默认）" name="response_header_timeout_seconds" type="number" min="0" onChange=${change}/>
    <${TextField} value=${get(draft,'max_conns_per_host',0)} label="上游并发连接上限（0 不限制）" name="max_conns_per_host" type="number" min="0" onChange=${change}/>
    <${MultiField} values=${draft.policy_ids || []} label="Policies" name="policy_ids" options=${policies} onChange=${change}/>
    <${CheckField} value=${get(draft,'enabled',true)} label="启用" name="enabled" onChange=${change}/>
    <button class="button button-primary" type="submit" disabled=${busy}>${submitLabel}</button>
  </form>`;
}
