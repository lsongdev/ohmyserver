import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { PageHeader, Alert, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import ProxyFields, { parseProxy } from './proxy-form.js';
import { WithConfig, sectionFor, entries, withEntries, format } from '../xray.js';

export default function Editor({ params }) {
  const key = params.section;
  const section = sectionFor(key);
  if (key && !section) return html`<${Alert} variant="warning">Page not found<//>`;
  const index = params.index === undefined ? null : Number(params.index);
  const back = section ? '/' + key : '/';
  const title = !section ? 'Configuration' : (index === null ? 'Add ' : 'Edit ') + section.item;
  return html`<${WithConfig} render=${state => {
      const list = section ? entries(state.config, key) : [];
      const original = !section ? state.config : index === null ? section.example
        : /^(0|[1-9]\d*)$/.test(params.index) ? list[index] : null;
      return original ? html`<${Form} state=${state} original=${original} title=${title} backLabel=${section?.title || 'Overview'} sectionKey=${key} list=${list} index=${index} back=${back} />`
        : html`<${PageHeader} title=${title} /><${Alert} variant="warning">This entry no longer exists.<//>`;
    }} />`;
}

const fields = {
  rules: [['ruleTag', 'Rule tag'], ['outboundTag', 'Outbound tag'], ['balancerTag', 'Balancer tag'], ['network', 'Network']],
};

function Form({ state, original, title, backLabel, sectionKey, list, index, back }) {
  const { api, href, navigate } = usePanel();
  const { busy, error, run } = useAction();
  const proxy = sectionKey === 'inbounds' || sectionKey === 'outbounds';
  const [mode, setMode] = useState('form');
  const [entry, setEntry] = useState(original);
  const editable = fields[sectionKey] || [];
  const [values, setValues] = useState(() => Object.fromEntries(editable.map(([name]) => [name, String(original[name] ?? '')])));
  const [text, setText] = useState(() => format(proxy ? original : Object.fromEntries(Object.entries(original).filter(([key]) => !editable.some(([name]) => name === key)))));
  const current = proxy && mode === 'form' ? format(entry) : JSON.stringify([values, text]);
  const [initial] = useState(() => proxy ? format(original) : current);
  const [parseError, setParseError] = useState('');
  let dirty = current !== initial;
  if (proxy && mode === 'json') {
    try { dirty = format(parseProxy(text)) !== initial; } catch { dirty = true; }
  }
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const switchMode = next => {
    if (next === mode) return;
    if (next === 'json') {
      setText(format(entry));
    } else {
      try { setEntry(parseProxy(text)); } catch (err) { setParseError(err.message); return; }
    }
    setParseError('');
    setMode(next);
  };
  const submit = event => {
    event.preventDefault();
    setParseError('');
    let data;
    try {
      data = proxy && mode === 'form' ? entry : parseProxy(text);
      if (proxy && mode === 'form' && ['socks', 'mixed'].includes(data.protocol)
          && data.settings?.auth === 'password' && !data.settings?.accounts?.length) {
        throw new Error('Add an account for password authentication.');
      }
      for (const [name] of editable) {
        const value = values[name].trim();
        if (value) data[name] = value;
        else delete data[name];
      }
    } catch (err) {
      setParseError(err.message);
      return;
    }
    const next = [...list];
    if (sectionKey) next[index ?? next.length] = data;
    run(async () => {
      await api.put('/config', { revision: state.revision, config: sectionKey ? withEntries(state.config, sectionKey, next) : data });
      toast('Configuration saved', 'success');
      saved();
      navigate(back, { replace: true });
    });
  };
  const actions = proxy && html`<div class="button-group" role="group" aria-label="Editor mode">
    <button type="button" class="button" disabled=${busy} aria-pressed=${mode === 'form'} onClick=${() => switchMode('form')}>Form</button>
    <button type="button" class="button" disabled=${busy} aria-pressed=${mode === 'json'} onClick=${() => switchMode('json')}>JSON</button>
  </div>`;
  return html`<${PageHeader} title=${title} subtitle="Save a draft, then apply it from Overview."
    actions=${actions} back=${proxy ? undefined : { href: href(back), label: backLabel }} />
  <form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${busy}>
      <${Alert} error=${error || (proxy && mode === 'form' ? parseError : null)} />
      ${proxy ? html`<${ProxyFields} sectionKey=${sectionKey} value=${entry} change=${next => { setEntry(next); setParseError(''); }}
        raw=${text} changeRaw=${next => { setText(next); setParseError(''); }} error=${parseError}
        mode=${mode} setMode=${switchMode} />` : html`
      <div class="xray-fields">${editable.map(([name, label]) => html`
        <${Field} key=${name} label=${label}>
          <input class="input" value=${values[name]}
            onInput=${e => setValues({ ...values, [name]: e.currentTarget.value })} />
        <//>`)}</div>
      <${Field} label=${sectionKey ? 'Additional settings (JSON)' : 'Xray configuration (JSON)'} error=${parseError} required>
        <textarea class="input xray-json" rows=${sectionKey ? 12 : 22} required spellcheck="false" value=${text}
          onInput=${e => { setText(e.currentTarget.value); setParseError(''); }} />
      <//>`}
      <div class="form-actions">
        <button class="button button-primary" type="submit">Save draft</button>
        <a class="button" href=${href(back)}>Cancel</a>
      </div>
    </fieldset>
  </form>`;
}
