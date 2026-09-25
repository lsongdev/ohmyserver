import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { matchModuleRoute, readLocation, scopeModulePath } from './router.js';

globalThis.location = { origin: 'http://localhost', hash: '' };

describe('hash router matching', () => {
  it('prefers static routes over an earlier parameter route', () => {
    const routes = [
      { path: '/certificates/:id', name: 'certificate' },
      { path: '/certificates/new', name: 'new-certificate' },
      { path: '/certificates/csr', name: 'csr' },
    ];
    assert.equal(matchModuleRoute(routes, '/certificates/new').route.name, 'new-certificate');
    assert.equal(matchModuleRoute(routes, '/certificates/csr').route.name, 'csr');
    assert.deepEqual(matchModuleRoute(routes, '/certificates/42').params, { id: '42' });
  });

  it('keeps encoded slashes inside a single route parameter', () => {
    const route = { path: '/items/:id', name: 'item' };
    assert.deepEqual(matchModuleRoute([route], '/items/a%2Fb').params, { id: 'a/b' });
    const location = readLocation('#/items/a%2Fb?tab=details');
    assert.deepEqual(location.rawSegments, ['items', 'a%2Fb']);
    assert.deepEqual(location.segments, ['items', 'a/b']);
    assert.equal(location.search.get('tab'), 'details');
  });
});

describe('module-scoped navigation', () => {
  it('maps module-relative paths into the host page mount', () => {
    assert.equal(scopeModulePath('/dhcp', '/leases'), '/dhcp/leases');
    assert.equal(scopeModulePath('/', '/'), '/');
  });

  it('rejects host-prefixed module paths instead of supporting compatibility mode', () => {
    assert.throws(() => scopeModulePath('/dhcp', '/dhcp/leases'), /relative to the module root/);
    assert.throws(() => scopeModulePath('/dhcp', 'leases'), /single \//);
  });
});
