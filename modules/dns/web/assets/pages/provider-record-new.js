import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';
import { decodeProviderZone } from '../provider-route.js';

const empty = { type: 'A', name: '', value: '', ttl: 300, comment: '' };
export default function NewProviderRecord({ api, params, navigate }) {
  const [zoneID, setZoneID] = useState(''), [zoneName, setZoneName] = useState(''), [form, setForm] = useState(empty), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  useEffect(() => {
    let decoded;
    try { decoded = decodeProviderZone(params.zone); } catch { setError('托管区域地址无效'); return; }
    setZoneID(decoded);
    const controller = new AbortController();
    api.listProviderZones(params.id, { signal: controller.signal }).then(zones => setZoneName((zones || []).find(item => item.id === decoded)?.name || decoded)).catch(e => { if (e.name !== 'AbortError') setError(e.message || '托管区域读取失败'); });
    return () => controller.abort();
  }, [api, params.id, params.zone]);
  const parent = `/dns/providers/${params.id}/zones/${params.zone}`;
  const set = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.createProviderRecord(params.id, zoneID, { ...form, ttl: Number(form.ttl) || 300 }); navigate(parent); } catch (e) { setError(e.message || '添加记录失败'); } finally { setBusy(false); } };
  return html`<section class="card"><div class="page-toolbar"><h2>添加 ${zoneName || 'DNS'} 记录</h2><button class="button" onClick=${() => navigate(parent)}>返回区域</button></div>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<form class="dns-record-form" onSubmit=${submit}><label class="form-field">类型<select class="input" value=${form.type} onChange=${e => set('type', e.currentTarget.value)}>${['A','AAAA','CNAME','TXT','MX','SRV','NS','PTR'].map(type => html`<option value=${type}>${type}</option>`)}</select></label><label class="form-field">名称<input class="input" value=${form.name} onInput=${e => set('name', e.currentTarget.value)} required /></label><label class="form-field">值<input class="input" value=${form.value} onInput=${e => set('value', e.currentTarget.value)} required /></label><label class="form-field">TTL<input class="input" type="number" min="1" value=${form.ttl} onInput=${e => set('ttl', e.currentTarget.value)} /></label><label class="form-field">备注<input class="input" value=${form.comment} onInput=${e => set('comment', e.currentTarget.value)} /></label><div class="form-actions"><button class="button button-primary" disabled=${busy || !zoneID}>${busy ? '添加中…' : '添加记录'}</button><button class="button" type="button" onClick=${() => navigate(parent)}>取消</button></div></form></section>`;
}
