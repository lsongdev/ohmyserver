import { html, useEffect, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { Page, Notice, Loading, Field, Check, Modal, toast } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { KINDS, Unprotected } from '../notify.js';

const LEVELS = [
  ['critical', 'Critical only'],
  ['warning', 'Warning and above'],
  ['info', 'Info and above'],
  ['debug', 'Everything'],
];

const kindOf = c => KINDS.findIndex(k => k.type === c.type && (k.type === 'telegram' || k.preset === c.preset));

export default function ChannelForm() {
  const { api, params, href } = usePanel();
  const id = params.id;
  const channel = useResource(signal => (id ? api.get(`/channels/${id}`, { signal }) : Promise.resolve(null)), [id]);
  const back = { href: href('/channels'), label: 'Channels' };
  return html`<${Page} title=${id ? (channel.data?.name || 'Channel') : 'Add channel'} subtitle=${id ? null : 'Events are sent to it from now on, according to its rule'} back=${back}>
    <${Unprotected} />
    <${Notice} error=${channel.error} />
    ${channel.data === undefined ? (channel.error ? null : html`<${Loading} />`) : html`<${Form} key=${id || 'new'} saved=${channel.data} onSaved=${channel.reload} />`}
  <//>`;
}

function Form({ saved, onSaved }) {
  const { api, navigate } = usePanel();
  const initial = {
    name: saved?.name || '', kind: saved ? Math.max(kindOf(saved), 0) : 0, url: '', token: '',
    chat_id: saved?.chat_id || '', min_level: saved?.min_level || 'warning',
    sources: (saved?.sources || []).join(', '), enabled: saved ? saved.enabled : true,
  };
  const [form, setForm] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [dialog, setDialog] = useState(false);
  const [tested, setTested] = useState(null);
  const save = useAction();
  const test = useAction();
  const set = (key, value) => { setForm(f => ({ ...f, [key]: value })); setTested(null); };
  const kind = KINDS[form.kind];
  const dirty = JSON.stringify(form) !== JSON.stringify(baseline);
  const release = useUnsavedChanges(dirty, { pending: save.busy });
  const sameKind = saved && kindOf(saved) === form.kind;

  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      const body = {
        name: form.name, type: kind.type, preset: kind.preset, url: form.url, token: form.token, chat_id: form.chat_id,
        min_level: form.min_level, enabled: form.enabled,
        sources: form.sources.split(',').map(s => s.trim()).filter(Boolean),
      };
      if (saved) {
        await api.put(`/channels/${saved.id}`, body);
        toast(`Saved ${form.name}`, 'success');
        release();
        navigate('/channels');
      } else {
        const created = await api.post('/channels', body);
        toast(`Added ${form.name}. Send a test to check it works.`, 'success');
        release();
        // Stay on it, to try it.
        navigate(`/channels/${created.id}`, { replace: true });
      }
    });
  };
  const sendTest = () => test.run(async () => {
    setTested(null);
    await api.post(`/channels/${saved.id}/test`);
    setTested(`Sent. If it did not arrive, check ${kind.check}.`);
    onSaved();
  });

  return html`<form class="form settings-form" onSubmit=${submit}>
    <${Notice} error=${save.error} />
    <div class="card">
      <div class="form-fields">
        <${Field} label="Send to">
          <select class="input" autofocus=${!saved} disabled=${save.busy} value=${form.kind} onChange=${e => set('kind', Number(e.currentTarget.value))}>
            ${KINDS.map((k, i) => html`<option value=${i}>${k.title}</option>`)}
          </select>
        <//>
        ${kind.type === 'telegram' ? html`<${TelegramFields} form=${form} set=${set} saved=${sameKind ? saved : null} busy=${save.busy} />` : html`
          <${Field} label="Address" required=${!sameKind} hint=${sameKind ? `Saved as ${saved.target}. Leave empty to keep it.` : kind.hint}>
            <input class="input mono" type="password" autocomplete="off" required=${!sameKind} placeholder=${kind.placeholder} disabled=${save.busy} value=${form.url} onInput=${e => set('url', e.currentTarget.value)} />
          <//>
          ${kind.token ? html`<${Field} label="Bearer token" hint=${sameKind && saved.has_token ? 'Saved. Leave empty to keep it.' : 'Optional'}>
            <input class="input mono" type="password" autocomplete="off" disabled=${save.busy} value=${form.token} onInput=${e => set('token', e.currentTarget.value)} />
          <//>` : null}`}
        <${Field} label="Send" hint="A resolved problem is announced wherever it was sent">
          <select class="input" disabled=${save.busy} value=${form.min_level} onChange=${e => set('min_level', e.currentTarget.value)}>
            ${LEVELS.map(([value, title]) => html`<option value=${value}>${title}</option>`)}
          </select>
        <//>
        <${Field} label="From" hint="Sources such as zfs, cron. Empty means all">
          <input class="input mono" autocomplete="off" spellcheck="false" placeholder="All sources" disabled=${save.busy} value=${form.sources} onInput=${e => set('sources', e.currentTarget.value)} />
        <//>
        <${Field} label="Name" required hint="To tell channels apart">
          <input class="input" required autocomplete="off" disabled=${save.busy} value=${form.name} onInput=${e => set('name', e.currentTarget.value)} />
        <//>
        <${Check} label="Turned on" checked=${form.enabled} disabled=${save.busy} onChange=${e => set('enabled', e.currentTarget.checked)} />
      </div>
    </div>
    <${Notice} error=${test.error} />
    ${tested ? html`<${Notice} tone="success">${tested}<//>` : null}
    <div class="notify-actions">
      <button class="button button-primary" type="submit" disabled=${save.busy || (saved && !dirty)}>${save.busy ? 'Saving…' : saved ? 'Save' : 'Add channel'}</button>
      <button class="button" type="button" disabled=${save.busy} onClick=${() => navigate('/channels')}>Cancel</button>
      ${saved ? html`
        <button class="button" type="button" disabled=${test.busy || dirty} title=${dirty ? 'Save first' : null} onClick=${sendTest}>${test.busy ? 'Sending…' : 'Send test'}</button>
        <span class="spacer"></span>
        <button class="button button-danger" type="button" onClick=${() => setDialog(true)}>Delete</button>` : null}
    </div>
    ${dialog ? html`<${DeleteDialog} channel=${saved} onClose=${() => setDialog(false)} onDeleted=${() => { release(); navigate('/channels'); }} />` : null}
  </form>`;
}

const WAIT = 3000;
const GIVE_UP = 100; // about five minutes of asking

// TelegramFields finds the chat instead of asking for its id: a pasted token is
// checked and names the bot, and whoever writes to the bot shows up to pick from.
// Typing the id stays possible, for a bot something else already reads.
function TelegramFields({ form, set, saved, busy }) {
  const { api } = usePanel();
  const [found, setFound] = useState(null);
  const [error, setError] = useState(null);
  const [asking, setAsking] = useState(false); // on a saved token, once asked to
  const token = form.token.trim();
  const canAsk = token.includes(':') || (!token && saved?.has_token && asking);
  const chosen = form.chat_id.trim();
  const body = { token, channel: token ? 0 : saved?.id || 0 };
  const key = JSON.stringify(body);

  // Waits a moment after typing, then looks every few seconds for about five
  // minutes, also after a chat is chosen: others may still write, and this stops
  // with the page.
  useEffect(() => {
    setFound(null);
    setError(null);
    if (!canAsk) return undefined;
    const controller = new AbortController();
    let timer;
    let tries = 0;
    const look = async () => {
      try {
        const next = await api.post('/telegram/discover', body, { signal: controller.signal });
        setFound(next);
        setError(null);
        if (!next.problem && ++tries < GIVE_UP) timer = setTimeout(look, WAIT);
      } catch (err) {
        if (err.name !== 'AbortError') setError(err);
      }
    };
    timer = setTimeout(look, token ? 500 : 0);
    return () => { controller.abort(); clearTimeout(timer); };
  }, [key, canAsk]);

  const pick = chat => {
    set('chat_id', chat.id);
    if (!form.name.trim()) set('name', `Telegram: ${chat.title}`);
  };
  const username = found?.bot?.username;
  return html`
    <${Field} label="Bot token" required=${!saved} error=${error?.message}
      hint=${saved?.has_token ? 'Saved. Leave empty to keep it.' : html`Create a bot with <a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer">@BotFather</a> and paste its token here.`}>
      <input class="input mono" type="password" autocomplete="off" required=${!saved} disabled=${busy} value=${form.token} onInput=${e => set('token', e.currentTarget.value)} />
    <//>
    ${saved?.has_token && !token && !asking ? html`<div><button class="button button-small" type="button" onClick=${() => setAsking(true)}>Find another chat</button></div>` : null}
    ${canAsk && !found && !error ? html`<p class="muted small notify-step">Checking the token…</p>` : null}
    ${found && username ? html`<div class="notify-telegram">
      <strong>Connected to <a href=${`https://t.me/${username}`} target="_blank" rel="noopener noreferrer">@${username}</a></strong>
      ${found.problem ? html`<${Notice}>${found.problem}<//>` : html`
        <p class="notify-step">
          Open <a href=${`https://t.me/${username}`} target="_blank" rel="noopener noreferrer">@${username}</a> and send it any message.
          For a group, add the bot to it and send a message that mentions it.
        </p>
        ${found.chats.length === 0 ? html`<p class="muted small notify-step">Waiting for a message…</p>` : html`
          <div class="notify-chats" role="radiogroup" aria-label="Chat to send to">
            ${found.chats.map(c => html`<label class=${`notify-chat${chosen === c.id ? ' selected' : ''}`} key=${c.id}>
              <input type="radio" name="telegram-chat" disabled=${busy} checked=${chosen === c.id} onChange=${() => pick(c)} />
              <span><span class="cell-main">${c.title}</span> <span class="muted small">${CHAT_TYPES[c.type] || c.type}</span></span>
            </label>`)}
          </div>
          <p class="muted small notify-step">Not listed? Send it a message, and it appears here.</p>`}`}
    </div>` : null}
    <${Field} label="Chat" required hint="Filled in when you pick a chat above; you can also type its ID">
      <input class="input mono" required autocomplete="off" spellcheck="false" disabled=${busy} value=${form.chat_id} onInput=${e => set('chat_id', e.currentTarget.value)} />
    <//>`;
}

const CHAT_TYPES = { private: 'Direct message', group: 'Group', supergroup: 'Group', channel: 'Channel' };

function DeleteDialog({ channel, onClose, onDeleted }) {
  const { api } = usePanel();
  const { busy, error, run } = useAction();
  const close = () => {
    if (busy) return false;
    onClose();
  };
  const submit = () => run(async () => {
    await api.del(`/channels/${channel.id}`);
    toast(`Deleted ${channel.name}`, 'success');
    onClose();
    onDeleted();
  });
  return html`<${Modal} title=${`Delete ${channel.name}?`} onClose=${close} footer=${html`
    <button class="button" type="button" autofocus disabled=${busy} onClick=${close}>Cancel</button>
    <button class="button button-danger" type="button" disabled=${busy} onClick=${submit}>${busy ? 'Deleting…' : 'Delete'}</button>`}>
    <${Notice} error=${error} />
    <p style="margin:0">It stops receiving events, its address and token are forgotten, and what is still waiting to be sent to it is dropped.</p>
  <//>`;
}
