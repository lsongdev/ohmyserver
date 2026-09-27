import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

export default function Storage({ api }) {
  const [volumes,setVolumes]=useState([]),[devices,setDevices]=useState([]),[device,setDevice]=useState(''),[health,setHealth]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=async signal=>{setError('');try{const [usage,list]=await Promise.all([api.usage({signal}),api.devices({signal})]);setVolumes(usage||[]);setDevices(list||[]);if(!device&&list?.length)setDevice(list[0]);}catch(e){if(e.name!=='AbortError')setError(e.message||'磁盘信息读取失败');}};
  useEffect(()=>{const controller=new AbortController();load(controller.signal);return()=>controller.abort();},[api]);
  const inspect=async()=>{if(!device)return;setBusy(true);setError('');try{setHealth(await api.health(device));}catch(e){setHealth(null);setError(e.message||'SMART 信息读取失败');}finally{setBusy(false);}};
  const percent=value=>`${Math.max(0,Math.min(100,Number(value)||0)).toFixed(1)}%`;
  return html`<div class="storage-page">
    ${error&&html`<div class="notice-card error" role="alert">${error}</div>`}
    <section class="card"><div class="page-toolbar"><h2>文件系统用量</h2><button class="button" onClick=${()=>load()}>刷新</button></div>
      ${volumes.length?html`<div class="table-wrap"><table class="table"><thead><tr><th>设备</th><th>挂载点</th><th>文件系统</th><th>已用</th><th>可用</th><th>总量</th><th>使用率</th></tr></thead><tbody>${volumes.map((item,i)=>html`<tr key=${`${item.device}:${item.mount_point}:${i}`}><td class="mono">${item.device}</td><td class="mono">${item.mount_point}</td><td>${item.fs_type}</td><td>${item.used_gb.toFixed(2)} GB</td><td>${item.free_gb.toFixed(2)} GB</td><td>${item.total_gb.toFixed(2)} GB</td><td>${percent(item.use_percent)}</td></tr>`)}</tbody></table></div>`:html`<p class="muted">没有可显示的文件系统。</p>`}
    </section>
    <section class="card"><div class="page-toolbar"><div><h2>SMART 健康信息</h2><p class="muted">仅查询系统枚举出的块设备；执行需要系统已安装并授权 smartctl。</p></div><div class="button-group"><select class="input" aria-label="磁盘设备" value=${device} onChange=${e=>{setDevice(e.currentTarget.value);setHealth(null);}}>${devices.map(item=>html`<option value=${item}>${item}</option>`)}</select><button class="button button-primary" disabled=${busy||!device} onClick=${inspect}>${busy?'读取中…':'读取 SMART'}</button></div></div>
      ${health&&html`<div class="storage-health"><div class="page-toolbar"><h3 class="mono">${health.device} · ${health.model_name||'未知型号'}</h3><span class=${`status-pill ${health.healthy?'running':''}`}>${health.healthy?'健康':'需检查'}</span></div>${health.smart_error&&html`<div class="notice-card error">${health.smart_error}</div>`}<div class="storage-health-grid"><div><span>序列号</span><strong>${health.serial||'—'}</strong></div><div><span>温度</span><strong>${health.temperature||'—'}</strong></div><div><span>容量</span><strong>${health.capacity||'—'}</strong></div><div><span>通电时间</span><strong>${health.power_on_hours||'—'}</strong></div></div>${health.nvme_health&&html`<p>${health.nvme_health}</p>`}${health.scsi_error&&html`<div class="notice-card error">${health.scsi_error}</div>`}${health.attributes?.length?html`<h3>ATA SMART 属性</h3><div class="table-wrap"><table class="table"><thead><tr><th>ID</th><th>属性</th><th>当前</th><th>最差</th><th>阈值</th><th>失败标记</th><th>原始值</th></tr></thead><tbody>${health.attributes.map((x,i)=>html`<tr key=${`${x.id}:${i}`}><td>${x.id}</td><td>${x.name}</td><td>${x.value}</td><td>${x.worst}</td><td>${x.threshold}</td><td>${x.when_failed||'—'}</td><td class="mono">${x.raw}</td></tr>`)}</tbody></table></div>`:null}</div>`}
    </section>
  </div>`;
}
