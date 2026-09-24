import test from 'node:test';
import assert from 'node:assert/strict';
import { pageForRoute, pageKey } from './page-state.js';

test('a lazy page is only reused for the route key that loaded it', () => {
  const dockerPage = () => 'docker';
  const loaded = { key: 'docker:/containers', component: dockerPage };

  assert.equal(pageForRoute(loaded, pageKey({ id: 'docker' }, { path: '/containers' })), dockerPage);
  assert.equal(pageForRoute(loaded, pageKey({ id: 'dashboard' }, { path: '/' })), null);
  assert.equal(pageForRoute(loaded, pageKey({ id: 'docker' }, { path: '/images' })), null);
  assert.equal(pageForRoute(loaded, ''), null);
});
