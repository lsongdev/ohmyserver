// Notification center helpers shared by the pages.
import { html } from './lib/html.js';
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Notice } from './lib/ui.js';

// What a channel can send to. Group bots and the like are plain web hooks.
export const KINDS = [
  { type: 'webhook', preset: 'feishu', title: 'Feishu group bot', hint: 'A custom bot’s web hook address, from the group’s settings', placeholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/…', check: 'the bot is still in the group' },
  { type: 'webhook', preset: 'wecom', title: 'WeCom group bot', hint: 'A group bot’s web hook address', placeholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=…', check: 'the bot is still in the group' },
  { type: 'telegram', preset: '', title: 'Telegram', check: 'the bot can post in that chat' },
  { type: 'webhook', preset: 'bark', title: 'Bark', hint: 'Your device’s address from the Bark app', placeholder: 'https://api.day.app/your-key', check: 'the device key' },
  { type: 'webhook', preset: 'ntfy', title: 'ntfy', hint: 'The address of the topic', placeholder: 'https://ntfy.sh/your-topic', token: true, check: 'you are subscribed to the topic' },
  { type: 'webhook', preset: 'json', title: 'Web hook (JSON)', hint: 'Gets each message as JSON, with source, level, kind, title, body and time', placeholder: 'https://example.com/hook', token: true, check: 'what your service received' },
];

export function channelKind(c) {
  return KINDS.find(k => k.type === c.type && (k.type === 'telegram' || k.preset === c.preset))?.title || c.type;
}

const RULES = { critical: 'Critical only', warning: 'Warning and above', info: 'Info and above', debug: 'Everything' };

export function ruleText(c) {
  return `${RULES[c.min_level] || c.min_level}${c.sources.length ? ` from ${c.sources.join(', ')}` : ''}`;
}

// Unprotected warns where secrets are entered if the panel has no sign-in.
export function Unprotected() {
  const { api } = usePanel();
  const info = useResource(signal => api.get('/info', { signal }), []);
  return info.data?.unauthenticated ? html`<${Notice} tone="warn">This panel has no password: anyone who can reach it can change where notifications go, and send them as anyone. Set a password before relying on this.<//>` : null;
}
