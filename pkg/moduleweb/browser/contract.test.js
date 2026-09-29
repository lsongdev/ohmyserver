import test from 'node:test';
import assert from 'node:assert/strict';
import { discoverModule, validateManifest, validateMount, validateRuntime } from './definition.js';
import { createModuleTransport, createRequest } from './transport.js';
import { matchModuleRoute, navigationFor, resolveLocation } from './router.js';
import { loadPage } from './view.js';

globalThis.location = { origin: 'https://example.test', hash: '' };
const manifest = { contractVersion: 1, id: 'demo', entry: 'assets/module.js', home: '/', styles: [], navigation: [] };
const definition = () => validateManifest(manifest);
const runtime = () => ({ id: 'demo', createAPI: () => ({}), routes: [{ path: '/', load: async () => ({ default() {} }) }] });

test('manifest identity does not depend on its HTTP mount', async () => {
  for (const apiBase of ['/', '/v1/demo', '/panel/tools/proxy']) {
    const calls = [];
    const result = await discoverModule({ apiBase, pageBase: '/infra/service', fetchImpl: async url => { calls.push(url); return Response.json(manifest); } });
    const root = `https://example.test${apiBase === '/' ? '/' : apiBase + '/'}`;
    assert.deepEqual(calls, [root + 'manifest.json']);
    assert.equal(result.manifest.id, 'demo');
    assert.equal(result.mount.apiBase, apiBase);
    assert.equal(result.entryURL, root + 'assets/module.js');
    assert.equal(result.mount.pageBase, '/infra/service');
  }
});

test('manifest cannot introduce host mount or runtime fields', () => {
  for (const field of ['apiBase', 'pageBase', 'manifestURL', 'routes', 'createAPI', 'mount']) {
    assert.throws(() => validateManifest({ ...manifest, [field]: '/elsewhere' }), /unknown/);
  }
});

test('host mount descriptors expose only the fixed three fields', () => {
  assert.deepEqual(validateMount({ id: 'demo', apiBase: '/service', pageBase: '/tools/demo' }), {
    id: 'demo', apiBase: '/service', pageBase: '/tools/demo',
  });
  assert.throws(() => validateMount({ id: 'demo', apiBase: '/service', pageBase: '/demo', manifestURL: '/elsewhere' }), /unknown/);
});

test('invalid definitions fail before rendering', () => {
  for (const change of [{ contractVersion: 99 }, { id: '../bad' }, { entry: undefined }, { entry: '../other.js' }, { home: '//evil' }, { styles: ['https://elsewhere/style.css'] }, { navigation: [null] }]) {
    assert.throws(() => validateManifest({ ...manifest, ...change }));
  }
  for (const key of ['api', 'params', 'search', 'href', 'navigate', 'hrefTo']) {
    const value = runtime(); value.routes[0].props = { [key]: null };
    assert.throws(() => validateRuntime(value, definition()), /conflict/);
  }
  const value = runtime();
  value.routes.push({ path: '/items/:id', load() {} }, { path: '/items/:name', load() {} });
  assert.throws(() => validateRuntime(value, definition()), /duplicate/);
  assert.throws(() => validateRuntime(runtime(), validateManifest({ ...manifest, home: '/missing' })), /no route/);
});

test('resolves host-assigned page mounts by longest segment boundary', () => {
  const routes = [{ path: '/', load() {} }, { path: '/items/:id', load() {} }];
  const root = { mount: { id: 'root', pageBase: '/' }, manifest: { home: '/' }, runtime: { routes } };
  const nested = { mount: { id: 'demo', pageBase: '/infra/proxy' }, manifest: { home: '/' }, runtime: { routes } };
  const exact = resolveLocation('#/infra/proxy/items/a%2Fb?q=one', [root, nested]);
  assert.equal(exact.module, nested);
  assert.equal(exact.params.id, 'a/b');
  assert.equal(exact.location.search.get('q'), 'one');
  assert.equal(resolveLocation('#/items/42', [root, nested]).module, root);
  assert.equal(resolveLocation('#/infra/proxy-other', [nested]).module, null);
  assert.equal(resolveLocation('#/infra/proxy', [root, nested]).page.path, '/');
  assert.equal(navigationFor('/infra/proxy').href('/items/a%2Fb'), '#/infra/proxy/items/a%2Fb');
  assert.equal(navigationFor('/').hrefTo('docker', '/containers'), null);
});

test('static route priority is positional and independent of registration order', () => {
  const routes = [{ path: '/:id/edit' }, { path: '/new/:tab' }];
  assert.equal(matchModuleRoute(routes, '/new/edit').route.path, '/new/:tab');
  assert.equal(matchModuleRoute([...routes].reverse(), '/new/edit').route.path, '/new/:tab');
});

test('all transport capabilities reject paths escaping the module namespace', () => {
  for (const apiBase of ['/', '/panel/proxy']) {
    const transport = createModuleTransport({ apiBase });
    for (const bad of ['items', '//other.test', 'https://other.test', '/../x', '/%2e%2e/x', '/x/../y', '/x#fragment', '/\\evil']) {
      for (const capability of ['request', 'fetchImpl', 'apiURL', 'websocketURL']) assert.throws(() => transport[capability](bad), TypeError);
    }
    assert.match(transport.apiURL('/items/a%2Fb?x=1'), /items\/a%2Fb\?x=1$/);
  }
});

test('request preserves null, false, zero and caller cancellation with injected fetch', async () => {
  for (const data of [null, false, 0, '', []]) {
    const controller = new AbortController();
    const transport = createModuleTransport({ apiBase: '/alias', fetchImpl: async (url, options) => {
      assert.equal(url, 'https://example.test/alias/items');
      assert.equal(options.signal, controller.signal);
      return Response.json({ data });
    } });
    assert.deepEqual(await transport.request('/items', { signal: controller.signal }), data);
  }
  const request = createRequest({ fetchImpl: async () => Response.json({ unexpected: true }) });
  await assert.rejects(request('/'), error => error.code === 'invalid_response');
});

test('errors preserve module status and code; raw fetch preserves streams', async () => {
  const transport = createModuleTransport({ apiBase: '/', fetchImpl: async () => Response.json({ error: { code: 'record_missing', message: 'Gone' } }, { status: 404 }) });
  await assert.rejects(transport.request('/items/1'), error => error.status === 404 && error.code === 'record_missing' && error.message === 'Gone');
  const response = new Response('data: ready\n\n', { headers: { 'content-type': 'text/event-stream' } });
  const streaming = createModuleTransport({ apiBase: '/', fetchImpl: async () => response });
  assert.equal(await streaming.fetchImpl('/events'), response);
  assert.equal(response.bodyUsed, false);
});

test('page loading is shared, deduplicated and retried after failure', async () => {
  let calls = 0;
  const Page = () => null;
  const route = { load: async () => { if (++calls === 1) throw new Error('offline'); return { default: Page }; } };
  await assert.rejects(loadPage(route), /offline/);
  const [first, second] = await Promise.all([loadPage(route), loadPage(route)]);
  assert.equal(first, Page); assert.equal(second, Page); assert.equal(calls, 2);
});
