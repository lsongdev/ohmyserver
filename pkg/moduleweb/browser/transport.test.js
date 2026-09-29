import test from 'node:test';
import assert from 'node:assert/strict';
import { createModuleTransport } from './transport.js';

test('scopes module requests and secondary URLs under the host assigned API mount', async () => {
  const calls = [];
  const transport = createModuleTransport({
    apiBase: '/panel/v1/docker',
    origin: 'https://panel.example',
    request: async (url, options) => { calls.push([url, options]); return 'ok'; },
    fetchImpl: async (url, options) => { calls.push([url, options]); return 'fetched'; },
  });

  assert.equal(await transport.request('/containers?state=running', { credentials: 'omit' }), 'ok');
  assert.equal(await transport.fetchImpl('/images/pull', { credentials: 'include' }), 'fetched');
  assert.equal(transport.apiURL('/containers/abc/logs'), 'https://panel.example/panel/v1/docker/containers/abc/logs');
  assert.equal(transport.websocketURL('/local/ws'), 'wss://panel.example/panel/v1/docker/local/ws');
  assert.deepEqual(calls, [
    ['https://panel.example/panel/v1/docker/containers?state=running', { credentials: 'same-origin' }],
    ['https://panel.example/panel/v1/docker/images/pull', { credentials: 'same-origin' }],
  ]);
});

test('rejects module requests that escape the assigned API mount', () => {
  const transport = createModuleTransport({ apiBase: '/v1/docker', origin: 'https://panel.example' });
  assert.throws(() => transport.apiURL('/../../admin'), /escaped its mount/);
  assert.throws(() => transport.apiURL('https://other.example/path'), /single \//);
});
