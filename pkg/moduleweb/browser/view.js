import { loadRuntime } from './definition.js';
import { createModuleTransport } from './transport.js';
import { navigationFor, resolveLocation } from './router.js';

const pageLoads = new WeakMap();
export function loadPage(route) {
  if (!pageLoads.has(route)) {
    const pending = Promise.resolve().then(() => route.load()).then(loaded => {
      if (typeof loaded.default !== 'function') throw new TypeError('module page must export a component');
      return loaded.default;
    }).catch(error => { pageLoads.delete(route); throw error; });
    pageLoads.set(route, pending);
  }
  return pageLoads.get(route);
}

// UI primitives are injected once by each host, so pages and their renderer
// always share the same Preact instance. Neither layer knows the host mode.
export function createModuleView({ html, Component, useEffect, useMemo, useState }) {
  class PageBoundary extends Component {
    constructor(props) { super(props); this.state = { error: null }; }
    static getDerivedStateFromError(error) { return { error }; }
    render() {
      return this.state.error
        ? html`<div class="notice-card error" role="alert"><p>${this.state.error.message}</p><button class="button" onclick=${() => this.setState({ error: null })}>重试</button></div>`
        : this.props.children;
    }
  }

  function PageView({ route, api, params, search, navigation }) {
    const [loaded, setLoaded] = useState(null);
    const [error, setError] = useState('');
    const [retry, setRetry] = useState(0);
    useEffect(() => {
      let active = true;
      setError('');
      loadPage(route).then(Page => { if (active) setLoaded(() => Page); }).catch(error => { if (active) setError(error.message); });
      return () => { active = false; };
    }, [route, retry]);
    if (error) return html`<div class="notice-card error" role="alert"><p>${error}</p><button class="button" onclick=${() => setRetry(value => value + 1)}>重试</button></div>`;
    if (!loaded) return html`<div class="loading-state" role="status">加载中…</div>`;
    return html`<${PageBoundary}><${loaded} ...${route.props} api=${api} params=${params} search=${search} ...${navigation} /><//>`;
  }

  function ModuleView({ definition, hash, hrefTo }) {
    const [runtime, setRuntime] = useState(null);
    const [error, setError] = useState('');
    const [retry, setRetry] = useState(0);
    useEffect(() => {
      let active = true;
      setError('');
      loadRuntime(definition).then(value => { if (active) setRuntime(value); }).catch(error => { if (active) setError(error.message); });
      return () => { active = false; };
    }, [definition, retry]);
    const api = useMemo(() => runtime?.createAPI(createModuleTransport({ apiBase: definition.mount.apiBase })), [runtime, definition]);
    const navigation = useMemo(() => navigationFor(definition.mount.pageBase, hrefTo), [definition, hrefTo]);
    const current = resolveLocation(hash, [{ ...definition, runtime }]);
    if (error) return html`<div class="notice-card error" role="alert"><p>${error}</p><button class="button" onclick=${() => setRetry(value => value + 1)}>重试</button></div>`;
    if (!runtime) return html`<div class="loading-state" role="status">正在加载模块…</div>`;
    if (!current.page) return html`<div class="notice-card">模块中没有此页面</div>`;
    return html`<${PageView} key=${current.page.path} route=${current.page} api=${api} params=${current.params} search=${current.location.search} navigation=${navigation} />`;
  }

  return function ModuleBoundary(props) {
    const { definition } = props;
    const key = `${definition.entryURL}:${definition.mount.apiBase}:${definition.mount.pageBase}`;
    return html`<${PageBoundary} key=${key}><${ModuleView} ...${props} /><//>`;
  };
}
