import { createAPI } from './api.js';
export default { id: 'speedtest', createAPI,
  routes: [{ path: '/', load: () => import('./pages/index.js') }] };
