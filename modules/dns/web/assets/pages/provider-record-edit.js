import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';
import { decodeProviderZone } from '../provider-route.js';

const empty = { type: 'A', name: '', value: '', ttl: 300, comment: '' };

export default function EditProviderRecord({ api, params, navigate }) {
  const [zoneID, setZoneID] = useState(''), [zoneName, setZoneName] = useState(''), [form, setForm] = useState(empty);
  const [error, setError] = useState(''), [busy, setBusy] = useState(false), [loading, setLoading] = useState(true);
  const parent = `/providers/${params.id}/zones/${params.zone}`;

  useEffect(() => {
    let decoded;
    try { decoded = decodeProviderZone(params.zone); } catch { setError('托管区域地址无效'); setLoading(false); return; }
    setZoneID(decoded);
    const controller = new AbortController();
    Promise.all([
      api.listProviderZones(params.id, { signal: controller.signal }),
      api.listProviderRecords(params.id, decoded, { signal: controller.signal }),
    ]).then(([zones, records]) => {
      setZoneName((zones || []).find(item => item.id === decoded)?.name || decoded);
      const record = (records || []).find(item => String(item.id) === String(params.record));
      if (!record) throw new Error('DNS 记录不存在');
      setForm({ ...empty, ...record });
    }).catch(e => { if (e.name !== 'AbortError') setError(e.message || 'DNS 记录读取失败'); })
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, [api, params.id, params.zone, params.record]);

  const set = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => {
    event.preventDefault(); setBusy(true); setError('');
    try {
      await api.updateProviderRecord(params.id, zoneID, { ...form, id: params.record, ttl: Number(form.ttl) || 300 });
      navigate(parent);
    } catch (e) { setError(e.message || '保存记录失败'); } finally { setBusy(false); }
  };

  return html`<section class="card"><div class="page-toolbar"><h2>编辑 ${zoneName || 'DNS'} 记录</h2><button class="button" onClick=${() => navigate(parent)}>返回区域</button></div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    ${loading ? html`<div class="loading-state">正在读取 DNS 记录…</div>` : null}
    ${!loading && !error ? html`<form class="dns-record-form" onSubmit=${submit}>
      <label class="form-field">类型<select class="input" value=${form.type} onChange=${e => set('type', e.currentTarget.value)}>${['A','AAAA','CNAME','TXT','MX','SRV','NS','PTR'].map(type => html`<option value=${type}>${type}</option>`)}</select></label>
      <label class="form-field">名称<input class="input" value=${form.name} onInput=${e => set('name', e.currentTarget.value)} required /></label>
      <label class="form-field">值<input class="input" value=${form.value} onInput=${e => set('value', e.currentTarget.value)} required /></label>
      <label class="form-field">TTL<input class="input" type="number" min="1" value=${form.ttl} onInput=${e => set('ttl', e.currentTarget.value)} /></label>
      <label class="form-field">备注<input class="input" value=${form.comment || ''} onInput=${e => set('comment', e.currentTarget.value)} /></label>
      <div class="form-actions"><button class="button button-primary" disabled=${busy || !zoneID}>${busy ? '保存中…' : '保存更改'}</button><button class="button" type="button" onClick=${() => navigate(parent)}>取消</button></div>
    </form>` : null}
  </section>`;
}
