import { html, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { PageHeader, Alert, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import { WithConfig, sectionFor, entries, withEntries, format } from '../xray.js';

export default function Editor({ params }) {
  const key = params.section;
  const section = sectionFor(key);
  const { href } = usePanel();
  if (key && !section) return html`<${Alert} variant="warning">Page not found<//>`;
  const index = params.index === undefined ? null : Number(params.index);
  const back = section ? '/' + key : '/';
  const title = !section ? 'Configuration' : (index === null ? 'Add ' : 'Edit ') + section.item;
  return html`<${PageHeader} title=${title} subtitle="Save a draft, then apply it from Overview."
    back=${{ href: href(back), label: section?.title || 'Overview' }} />
    <${WithConfig} render=${state => {
      const list = section ? entries(state.config, key) : [];
      const original = !section ? state.config : index === null ? section.example
        : /^(0|[1-9]\d*)$/.test(params.index) ? list[index] : null;
      return original ? html`<${Form} state=${state} original=${original} sectionKey=${key} list=${list} index=${index} back=${back} />`
        : html`<${Alert} variant="warning">This entry no longer exists.<//>`;
    }} />`;
}

const fields = {
  inbounds: [['tag', 'Tag'], ['protocol', 'Protocol'], ['listen', 'Listen address'], ['port', 'Port (number, range, or environment reference)']],
  outbounds: [['tag', 'Tag'], ['protocol', 'Protocol']],
  rules: [['ruleTag', 'Rule tag'], ['outboundTag', 'Outbound tag'], ['balancerTag', 'Balancer tag'], ['network', 'Network']],
};

function Form({ state, original, sectionKey, list, index, back }) {
  const { api, href, navigate } = usePanel();
  const { busy, error, run } = useAction();
  const editable = fields[sectionKey] || [];
  const [values, setValues] = useState(() => Object.fromEntries(editable.map(([name]) => [name, String(original[name] ?? '')])));
  const [text, setText] = useState(() => format(Object.fromEntries(Object.entries(original).filter(([key]) => !editable.some(([name]) => name === key)))));
  const [initial] = useState(() => JSON.stringify([values, text]));
  const [parseError, setParseError] = useState('');
  const saved = useUnsavedChanges(JSON.stringify([values, text]) !== initial, { pending: busy });
  const submit = event => {
    event.preventDefault();
    setParseError('');
    let data;
    try {
      data = JSON.parse(text);
      if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error('Enter a JSON object.');
      for (const [name] of editable) {
        const value = values[name].trim();
        if (value) data[name] = name === 'port' && /^\d+$/.test(value) ? Number(value) : value;
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
  return html`<form class="form settings-form" onSubmit=${submit}>
    <fieldset class="form-fields" disabled=${busy}>
      <${Alert} error=${error} />
      <div class="xray-fields">${editable.map(([name, label]) => html`
        <${Field} key=${name} label=${label} required=${name === 'protocol'}>
          <input class="input" required=${name === 'protocol'} value=${values[name]}
            onInput=${e => setValues({ ...values, [name]: e.currentTarget.value })} />
        <//>`)}</div>
      <${Field} label=${sectionKey ? 'Additional settings (JSON)' : 'Xray configuration (JSON)'} error=${parseError} required>
        <textarea class="input xray-json" rows=${sectionKey ? 12 : 22} required spellcheck="false" value=${text}
          onInput=${e => { setText(e.currentTarget.value); setParseError(''); }} />
      <//>
      <div class="form-actions">
        <button class="button button-primary" type="submit">Save draft</button>
        <a class="button" href=${href(back)}>Cancel</a>
      </div>
    </fieldset>
  </form>`;
}
