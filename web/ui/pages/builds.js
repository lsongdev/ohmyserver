import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, BuildStatus, EVENTS, active, datetime, short, duration } from '../ci.js';

export default function Builds() {
  const { href } = usePanel();
  const builds = useApi('/builds', list => (list?.some(b => active(b.status)) ? 2000 : 10000));
  const repos = useApi('/repos');
  const name = id => repos.data?.find(r => r.id === id)?.name || `#${id}`;
  return html`<${Page} title="构建" subtitle="所有仓库最近的构建。">
    <${Notice} error=${builds.error} />
    ${builds.data === undefined ? html`<${Loading} />` : !builds.data.length ? html`<${Empty}>还没有构建。<//>` : html`
      <div class="table-wrap"><table class="table">
        <thead><tr><th>仓库</th><th>构建</th><th>状态</th><th>分支 · 提交</th><th>触发</th><th>时间</th><th class="num">耗时</th></tr></thead>
        <tbody>${builds.data.map(b => html`<tr key=${b.id}>
          <td><a href=${href(`/repos/${b.repo_id}`)}>${name(b.repo_id)}</a></td>
          <td><a class="cell-main" href=${href(`/builds/${b.id}`)}>#${b.number}</a></td>
          <td><${BuildStatus} status=${b.status} title=${b.error} /></td>
          <td><span class="mono">${b.branch}</span> ${b.commit ? html`<span class="commit muted">${short(b.commit)}</span>` : null}
            <div class="cell-sub message" title=${b.message}>${b.message}</div></td>
          <td>${EVENTS[b.event] || b.event}</td>
          <td class="nowrap muted">${datetime(b.created_at)}</td>
          <td class="num">${duration(b.started_at, b.finished_at)}</td>
        </tr>`)}</tbody>
      </table></div>`}
  <//>`;
}
