import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { discoverModules } from './modules.js';

const repositoryRoot = resolve(new URL('../..', import.meta.url).pathname);

function moduleWebLayout(moduleID) {
  const webRoot = resolve(repositoryRoot, 'modules', moduleID, 'web');
  const manifestRoot = existsSync(resolve(webRoot, 'manifest.json')) ? webRoot : resolve(webRoot, 'assets');
  const assetRoot = existsSync(resolve(webRoot, 'assets')) ? resolve(webRoot, 'assets') : webRoot;
  return { webRoot, manifestRoot, assetRoot };
}

test('discovers module runtime and manifest URLs from the Go host index', async () => {
  const calls = [];
  const fetchImpl = async url => {
    calls.push(url);
    if (url === '/v1/modules') return new Response(JSON.stringify({ modules: [{ id: 'demo', apiBase: '/v1/demo', pageBase: '/demo', manifestURL: '/v1/demo/manifest.json' }] }), { status: 200 });
    return new Response(JSON.stringify({ contractVersion: 1, id: 'demo', title: 'Demo', group: 'Tools', entry: 'assets/index.js', home: '/', styles: [], navigation: [] }), { status: 200 });
  };
  const [module] = await discoverModules(fetchImpl);
  assert.equal(module.id, 'demo');
  assert.equal(module.apiBase, '/v1/demo');
  assert.equal(module.pageBase, '/demo');
  assert.equal(module.moduleURL, new URL('assets/index.js', 'http://localhost/v1/demo/').href);
  assert.deepEqual(calls, ['/v1/modules', 'http://localhost/v1/demo/manifest.json']);
});

test('rejects a manifest whose identity differs from the Go registration', async () => {
  const fetchImpl = async url => url === '/v1/modules'
    ? new Response(JSON.stringify({ modules: [{ id: 'demo', apiBase: '/v1/demo', pageBase: '/demo', manifestURL: '/v1/demo/manifest.json' }] }), { status: 200 })
    : new Response(JSON.stringify({ contractVersion: 1, id: 'other', entry: 'assets/index.js' }), { status: 200 });
  const [module] = await discoverModules(fetchImpl);
  assert.match(module.manifestError, /身份或版本无效/);
});

test('isolates a broken module manifest while loading the remaining modules', async () => {
  const fetchImpl = async url => {
    if (url === '/v1/modules') return new Response(JSON.stringify({ modules: [
      { id: 'broken', apiBase: '/v1/broken', pageBase: '/broken', manifestURL: '/v1/broken/manifest.json' },
      { id: 'working', apiBase: '/v1/working', pageBase: '/working', manifestURL: '/v1/working/manifest.json' },
    ] }), { status: 200 });
    if (url.includes('/broken/')) return new Response('missing', { status: 404 });
    return new Response(JSON.stringify({ contractVersion: 1, id: 'working', title: 'Working', entry: 'assets/index.js', home: '/', styles: [], navigation: [] }), { status: 200 });
  };
  const [broken, working] = await discoverModules(fetchImpl);
  assert.match(broken.manifestError, /清单加载失败/);
  assert.equal(working.moduleURL, 'http://localhost/v1/working/assets/index.js');
});

test('rejects module asset paths that escape the module assets directory', async () => {
  const fetchImpl = async url => url === '/v1/modules'
    ? new Response(JSON.stringify({ modules: [{ id: 'demo', apiBase: '/v1/demo', pageBase: '/demo', manifestURL: '/v1/demo/manifest.json' }] }), { status: 200 })
    : new Response(JSON.stringify({ contractVersion: 1, id: 'demo', entry: '../other.js' }), { status: 200 });
  const [module] = await discoverModules(fetchImpl);
  assert.match(module.manifestError, /资源路径无效/);
});

test('every Go module has a matching manifest, embedded assets, and lazy page imports', () => {
  const moduleDirs = readdirSync(resolve(repositoryRoot, 'modules'), { withFileTypes: true }).filter(entry => entry.isDirectory());
  assert.ok(moduleDirs.length > 0);
  for (const dir of moduleDirs) {
    const { manifestRoot, assetRoot } = moduleWebLayout(dir.name);
    const manifest = JSON.parse(readFileSync(resolve(manifestRoot, 'manifest.json'), 'utf8'));
    assert.equal(manifest.id, dir.name);
    for (const url of [manifest.entry, ...(manifest.styles || [])]) {
      assert.ok(url.startsWith('assets/'), `${dir.name}: ${url} is served under /assets`);
      assert.ok(existsSync(resolve(assetRoot, url.slice('assets/'.length))), `${dir.name}: ${url} exists in embedded web FS`);
    }
    const source = readFileSync(resolve(assetRoot, manifest.entry.slice('assets/'.length)), 'utf8');
    for (const [, relativePage] of source.matchAll(/import\(['"](\.\/pages\/[^'"]+)['"]\)/g)) {
      assert.ok(existsSync(resolve(assetRoot, relativePage)), `${dir.name}: ${relativePage} exists`);
    }
  }
});

test('every module keeps one standalone index and a separate shell entry', () => {
  const moduleDirs = readdirSync(resolve(repositoryRoot, 'modules'), { withFileTypes: true }).filter(entry => entry.isDirectory());
  for (const dir of moduleDirs) {
    const { assetRoot } = moduleWebLayout(dir.name);
    const html = readFileSync(resolve(assetRoot, 'index.html'), 'utf8');
    const index = readFileSync(resolve(assetRoot, 'index.js'), 'utf8');
    const manifest = JSON.parse(readFileSync(resolve(assetRoot, 'manifest.json'), 'utf8'));
    assert.equal(manifest.entry, 'assets/module.js', `${dir.name} manifest must point to the shell entry`);
    assert.match(html, /assets\/index\.js/, `${dir.name} standalone HTML must load index.js`);
    assert.match(html, /https:\/\/lsong\.org\/stylesheets\/index\.css/, `${dir.name} must use the shared CSS`);
    assert.match(index, /https:\/\/lsong\.org\/scripts\/react\/index\.js/, `${dir.name} standalone JS must use shared Preact`);
    assert.equal(existsSync(resolve(assetRoot, 'standalone.js')), false, `${dir.name} must not duplicate its standalone entry`);
    assert.equal(existsSync(resolve(assetRoot, 'standalone.css')), false, `${dir.name} must not duplicate standalone styles`);
  }
});

test('every module API client starts with host-scoped transport capabilities', async () => {
  const moduleDirs = readdirSync(resolve(repositoryRoot, 'modules'), { withFileTypes: true }).filter(entry => entry.isDirectory());
  const transport = {
    request: async () => null,
    fetchImpl: async () => new Response('{}'),
    apiURL: path => `https://panel.example/panel/v1/module${path}`,
    websocketURL: path => `wss://panel.example/panel/v1/module${path}`,
  };
  for (const dir of moduleDirs) {
    const { assetRoot } = moduleWebLayout(dir.name);
    const apiRoot = existsSync(resolve(assetRoot, 'api.js')) ? assetRoot : webRoot;
    const source = resolve(apiRoot, 'api.js');
    const sourceText = readFileSync(source, 'utf8');
    assert.doesNotMatch(sourceText, /apiBase|\/v1\/[a-z]/, `${dir.name} API client must not know host prefixes`);
    const { createAPI } = await import(pathToFileURL(source));
    assert.equal(typeof createAPI, 'function', `${dir.name} exports createAPI`);
    assert.ok(createAPI(transport), `${dir.name} accepts the host transport`);
  }
});
