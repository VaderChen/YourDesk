'use strict';
const $ = (selector) => document.querySelector(selector);
document.querySelectorAll('a[href="https://github.com/VaderChen/YourDesk"], a[href="https://buymeacoffee.com/vaderchen"]').forEach(link => link.addEventListener('click', event => {
  if (typeof window.yourdeskOpenExternal !== 'function') return;
  event.preventDefault();
  action(() => window.yourdeskOpenExternal(link.href));
}));

// 裝置 ID 共用輸入格式；IP 在輸入分隔符號後保留原本形式。
function bindDeviceIDInput(input) {
  const format = () => {
    const original = input.value;
    if (!original) return;
    let value = original.normalize('NFKC').trim();
    const caret = input.selectionStart ?? original.length;
    if (/[.:[\]]/.test(value)) {
      // 使用者可能先輸入 IPv4／IPv6 的第一段，當時已自動加上 ID 格式。
      if (/^YD-/i.test(value)) {
        value = value.slice(3).replace(/-/g, '');
        input.value = value;
        input.setSelectionRange(value.length, value.length);
      }
      return;
    }
    if (!/^[a-z0-9\s-]*$/i.test(value)) return;
    const prefixed = /^YD-/i.test(value) || (/^YD/i.test(value) && value.replace(/[\s-]/g, '').length === 22);
    const prefixLength = prefixed ? (/^YD-/i.test(value) ? 3 : 2) : 0;
    const body = value.slice(prefixLength).replace(/[\s-]/g, '').toUpperCase();
    // 不截斷過長的貼上內容，以免悄悄改成另一個裝置 ID。
    if (body.length > 20) return;
    const formatted = body ? 'YD-' + body.match(/.{1,4}/g).join('-') : '';
    const before = original.normalize('NFKC').slice(0, caret).trimStart().slice(prefixLength).replace(/[\s-]/g, '').length;
    const count = Math.min(before, body.length);
    const position = body ? Math.min(formatted.length, 3 + count + Math.floor(Math.max(0, count - 1) / 4)) : 0;
    input.value = formatted;
    input.setSelectionRange(position, position);
  };
  input.addEventListener('input', event => { if (!event.isComposing) format(); });
  input.addEventListener('compositionend', format);
  input.addEventListener('keydown', event => {
    const value = input.value;
    const start = input.selectionStart;
    if (event.isComposing || start !== input.selectionEnd || !/^YD-[A-Z0-9-]*$/.test(value)) return;
    // 分隔線是格式的一部分；刪除鍵直接刪除相鄰資料，避免卡在分隔線。
    if (event.key === 'Backspace' && start > 3 && value[start - 1] === '-') {
      event.preventDefault();
      input.setRangeText('', start - 2, start, 'end');
      format();
    } else if (event.key === 'Delete' && value[start] === '-' && start >= 3) {
      event.preventDefault();
      input.setRangeText('', start, start + 2, 'end');
      format();
    } else if (event.key === 'Backspace' && start <= 3) {
      event.preventDefault();
      if (value === 'YD-') input.value = '';
    }
  });
}
document.querySelectorAll('#quick-room, #site-form input[name="room"]').forEach(bindDeviceIDInput);

const token = location.hash.slice(1) || sessionStorage.getItem('yourdesk-token') || '';
if (location.hash) {
  sessionStorage.setItem('yourdesk-token', token);
  history.replaceState(null, '', location.pathname);
}
let state = null;
let startupMainReady = false;
let startupOptimizationDismissed = false;
let startupOptimizationFinished = false;
function startupOptimizationPending() {
  return ['not-started', 'running'].includes(state?.hardwareDetection?.status);
}
function renderStartupOptimization() {
  const dialog = $('#startup-optimization-dialog');
  if (!startupOptimizationPending()) {
    if (state?.hardwareDetection) startupOptimizationFinished = true;
    if (dialog.open) dialog.close();
    return;
  }
  if (state?.passwordPrompt || connectionWait) {
    if (dialog.open) dialog.close();
    return;
  }
  if (!startupMainReady || startupOptimizationDismissed || startupOptimizationFinished || document.hidden || busy) return;
  if (!dialog.open && !document.querySelector('dialog[open]')) openDialog('#startup-optimization-dialog');
}
$('#startup-optimization-dialog').addEventListener('close', () => { startupOptimizationDismissed = true; });
let selectedGroup = '*';
let groupSaveQueue = Promise.resolve();
function rememberGroup(id) {
 selectedGroup = id;
 if(state?.preferences)state.preferences.selectedGroup=id;
 groupSaveQueue=groupSaveQueue.then(()=>api('preferences','PUT',{selectedGroup:id})).catch(error=>toast(error.message,true));
}

let busy = false;
let toastTimer;
let confirmAction = null;
let groupDrag = null;
let siteDrag = null;
let sitePresence = {};
const siteRecovery = new Map();
let passwordReminderDismissed = false;
let checkingPresence = false;
let connectionWait = null;
let pendingDiagnosticSite = null;
let lastStatePoll = 0;
let statePolling = false;
let lastNotice = '';
const text = (tag, value, className) => {
  const element = document.createElement(tag);
  element.textContent = value;
  if (className) element.className = className;
  return element;
};
const button = (label, className, action, title) => {
  const element = text('button', label, className);
  element.type = 'button';
  if (title) { element.dataset.tooltip = title; element.setAttribute('aria-label', title); }
  element.addEventListener('click', action);
  return element;
};
function toast(message, error = false, { warning = false, duration = 4500 } = {}) {
  if (error && document.querySelector('dialog[open]')) {
    const dialog = document.querySelector('dialog[open]');
    let warning = dialog.querySelector('.form-error');
    if (!warning) { warning = text('p', '', 'form-error'); warning.setAttribute('role', 'alert'); dialog.querySelector('form').append(warning); }
    warning.textContent = i18n.t(message);
    return;
  }
  clearTimeout(toastTimer);
  $('#notice').textContent = i18n.t(message);
  $('#notice').className = error ? 'error' : warning ? 'warning' : '';
  $('#notice').hidden = false;
  toastTimer = setTimeout(() => { $('#notice').hidden = true; }, duration);
}
async function api(path, method = 'GET', body) {
  const response = await fetch(`/api/${path}`, {
    method,
    headers: { 'X-YourDesk-Token': token, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(10000)
  });
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || i18n.t('操作未完成，請稍後重試。'));
  return result;
}
async function action(callback) {
  if (busy) return;
  busy = true;
  document.querySelectorAll('dialog[open] button[type="submit"]').forEach(el => { el.disabled = true; });
  try { await callback(); }
  catch (error) { toast(error.message, true); }
  finally {
    busy = false;
    document.querySelectorAll('button[type="submit"]').forEach(el => { el.disabled = false; });
    if (state) renderQuick();
  }
}
function openDialog(id) {
  // 使用者操作或其他重要對話框優先；不把背景偵測疊在它們上方。
  if (id !== '#startup-optimization-dialog' && $('#startup-optimization-dialog').open) {
    startupOptimizationDismissed = true;
    $('#startup-optimization-dialog').close();
  }
  const dialog = $(id);
  dialog.querySelectorAll('.form-error').forEach(el => el.remove());
  dialog.showModal();
  // 標題可供鍵盤查看泡泡，但初始焦點應落在實際操作欄位。
  const visible = el => !el.disabled && el.getClientRects().length > 0;
  const initial = [...dialog.querySelectorAll('[autofocus]')].find(visible)
    || [...dialog.querySelectorAll('input:not([type="hidden"]):not([type="checkbox"]):not([type="radio"]), select, textarea')].find(visible)
    || [...dialog.querySelectorAll('button')].find(visible);
  initial?.focus({ preventScroll: true });
}
async function save(library) {
  await api('library', 'PUT', library);
  state.library = library;
  renderLibrary();
  refreshPresence();
}
function groupName(id) {
  return state.library.groups.find(group => group.id === id)?.name || i18n.t('未分組');
}
// 本機與遠端站台皆依 Server 實際註冊結果顯示在線狀態。
function siteOnline(site) { const value=sitePresence[site?.id]; return value && typeof value==='object'?value.online:value; }
function siteCapability(site,name) { return sitePresence[site?.id]?.capabilities?.[name]; }
function applySiteCapabilities(card,site) {
 const recovering=siteRecovery.has(site?.id);
 const device=card.querySelector('.mini-device');
 if(device){
  device.classList.toggle('mode-recovering',recovering);
  device.setAttribute('aria-busy',String(recovering));
 }

 for(const [name,label] of [['desktop','此裝置沒有桌面環境，請改用命令列連線。'],['terminal','對方尚未支援命令列，請更新對方的 YourDesk 後再試。']]){
  const btn=card.querySelector(`[data-connect-mode="${name}"]`);if(!btn)continue;
  const capability=siteCapability(site,name);
  btn.disabled=capability===false||recovering;
  btn.classList.toggle('mode-recovering',recovering&&capability!==false);
  btn.setAttribute('aria-busy',String(recovering&&capability!==false));
  const available=!recovering && capability===true && siteOnline(site)===true && !Object.hasOwn(state.running,`viewer:${site.id}`);
  btn.classList.toggle('mode-available',available);
  btn.dataset.tooltip=i18n.t(recovering?'等待遠端恢復連線…':btn.disabled?label:name==='desktop'?'開啟桌面':'開啟命令列');
 }
 const diagnostic=card.querySelector('[data-connect-mode="diagnostics"]');if(diagnostic)diagnostic.disabled=recovering||siteCapability(site,'desktop')===false;
}
function renderDevice() {
 $('#stop-incoming').hidden=!state.incomingConnected;
  const { info, running } = state;
  $('#device-room').textContent = state.prelogin?.enabled && state.prelogin.room ? state.prelogin.room : info.room;
  $('#footer-version').textContent = info.version;
  $('#local-secret').value = state.prelogin?.enabled ? '' : info.secret;
 $('#local-secret').placeholder=state.prelogin?.enabled?i18n.t('服務模式使用啟用時的密碼'):'';
 $('#toggle-secret').disabled=!!state.prelogin?.enabled;
  $('#password-reminder').hidden = info.passwordNeedsChange !== 'true' || passwordReminderDismissed;
  const active = Object.hasOwn(running, 'host');
  $('#host-status').textContent = state.prelogin?.enabled ? i18n.t('未登入連線服務模式') : active ? i18n.t('一切就緒') : i18n.t('無法連線');
  $('.local-badge').dataset.tooltip = `${info.hostname} · ${info.platform} / ${info.architecture}`;

}
function renderLibrary() {
  const { groups, sites } = state.library;
  if(selectedGroup!=='*' && selectedGroup!=='' && !groups.some(group=>group.id===selectedGroup))rememberGroup('*');
  $('#groups').replaceChildren();
  const options = [{ id: '*', name: i18n.t('所有站台') }, ...groups, { id: '', name: i18n.t('未分組') }];
  for (const group of options) {
    const row = text('div', '', `group-row${selectedGroup === group.id ? ' active' : ''}`);
    const select = button('', 'group-button', () => { rememberGroup(group.id); renderLibrary(); });
    select.setAttribute('aria-current', selectedGroup === group.id ? 'true' : 'false');
    select.append(text('span', group.id === '*' ? '▦' : '▱'), text('span', group.name, 'group-name'), text('span', group.id === '*' ? sites.length : sites.filter(site => site.group === group.id).length, 'group-count'));
    row.append(select);
    if (group.id && group.id !== '*') {
      row.dataset.groupId = group.id;
      row.draggable = true;
      row.addEventListener('contextmenu', event => { event.preventDefault(); openGroupMenu(group, event.clientX, event.clientY, select); });
      select.addEventListener('keydown', event => {
        if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) {
          event.preventDefault(); const bounds = row.getBoundingClientRect(); openGroupMenu(group, bounds.left, bounds.bottom, select);
        }
      });
    }
    $('#groups').append(row);
  }
  $('#filter-name').textContent = selectedGroup === '*' ? i18n.t('所有站台') : groupName(selectedGroup);
  renderSites();
}
// 圖示共用一致的線寬與按鈕尺寸，名稱保留給提示與輔助閱讀。
function siteIconButton(icon, label, onClick, active = false) {
  const control = button('', `icon-button site-action${icon === 'delete' ? ' site-action-delete' : ''}${active ? ' active' : ''}`, onClick, label);
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  for (const [key, value] of Object.entries({ viewBox: '0 0 24 24', width: '18', height: '18', fill: 'none', stroke: 'currentColor', 'stroke-width': '1.8', 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' })) svg.setAttribute(key, value);
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('d', {
    speed: 'M4 18a9 9 0 1 1 16 0M12 13l5-5M5 13h2M12 5v2M17 13h2M10 18h4',
    delete: 'M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7',
    edit: 'M14 5l5 5M4 20l5-1L21 7l-5-5L4 14z',
    connect: 'M3 3h18v14H3zM12 17v4M8 21h8',
    terminal: 'M3 4h18v16H3zM6 8l4 4-4 4M13 16h5',
    stop: 'M6 6h12v12H6z'
  }[icon]);
  svg.append(path); control.append(svg);
  if (["connect","terminal","speed"].includes(icon)) control.dataset.connectMode=icon==="connect"?"desktop":icon==="speed"?"diagnostics":"terminal";
  return control;
}
function renderSites() {
  if (siteDrag) return;
  const query = $('#search').value.trim().toLocaleLowerCase();
  const sites = state.library.sites.filter(site => (selectedGroup === '*' || site.group === selectedGroup) && `${site.name} ${site.room} ${site.note}`.toLocaleLowerCase().includes(query));
  $('#filter-count').textContent = i18n.t(`${sites.length} 個站台`);
  $('#site-list').replaceChildren();
  if (!sites.length) {
    const empty = text('div', '', 'empty-state');
    empty.append(text('div', query ? '⌕' : '＋', 'empty-symbol'), text('h3', query ? i18n.t('沒有符合的站台') : i18n.t('讓你的工作空間，從一個站台開始')), text('p', query ? i18n.t('試試其他名稱、裝置 ID 或備註。') : i18n.t('新增遠端裝置，將常用的連線整理在這裡。')));
    empty.append(button(query ? i18n.t('清除搜尋') : i18n.t('＋ 新增第一個站台'), 'button', () => { if (query) { $('#search').value = ''; renderSites(); } else editSite(); }));
    $('#site-list').append(empty);
    return;
  }
  for (const site of sites) {
    const card = text('article', '', 'site-card');
    card.dataset.siteId = site.id;
    card.draggable = true;
    card.addEventListener('pointerdown', event => { card.draggable = !event.target.closest('button, input, a'); });
    card.addEventListener('dblclick', event => {
      if (event.target.closest('button, input, select, textarea, a') || siteDrag || busy || document.querySelector('dialog[open]')) return;
      if (siteRecovery.has(site.id)||Object.hasOwn(state.running, `viewer:${site.id}`)) return;
      event.preventDefault();
      chooseConnection(site);
    });
    const header = text('div', '', 'card-header');
    const title = text('div', '', 'card-title');
    const name = text('h3', site.name); name.dataset.tooltip = site.name;
    title.append(name, text('p', groupName(site.group)));
    const online = siteOnline(site);
    const device = text('span', '', `mini-device${online === true ? ' online' : ' offline'}`);
    device.dataset.tooltip = i18n.t(online === true ? 'Client 在線上' : online === false ? 'Client 不在線上' : '無法確認 Client 狀態');
    const screen = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    for (const [key, value] of Object.entries({ viewBox: '0 0 24 24', width: '20', height: '20', fill: 'none', stroke: 'currentColor', 'stroke-width': '1.8', 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' })) screen.setAttribute(key, value);
    const outline = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    outline.setAttribute('d', 'M4 3h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1ZM12 17v4M8 21h8');
    screen.append(outline); device.append(screen);
    header.append(device, title);
    const footer = text('div', '', 'card-footer');
    const active = Object.hasOwn(state.running, `viewer:${site.id}`);
    footer.append(
      siteIconButton('delete', i18n.t(`刪除 ${site.name}`), () => deleteSite(site)),
      siteIconButton('edit', i18n.t(`編輯 ${site.name}`), () => editSite(site)),
      siteIconButton('terminal', i18n.t('開啟命令列'), () => active ? toast(i18n.t('請先關閉目前連線。'),true) : connect(site,true)),
      siteIconButton(active ? 'stop' : 'connect', active ? i18n.t('關閉連線') : i18n.t('開啟桌面'), () => active ? stop(`viewer:${site.id}`) : connect(site), active)
    );
    const note = text('p', site.note || '', 'card-note'); note.dataset.tooltip = site.note;
    const identity = text('div', '', 'card-id');
    identity.append(siteIconButton('speed', i18n.t('連線測速與分析'), () => openDiagnostics(site)), text('code', site.room));
    card.append(header, identity, note, footer);
    applySiteCapabilities(card,site);
    $('#site-list').append(card);
  }
}
function editSite(site) {
  if (!state) return;
  const form = $('#site-form');
  form.reset();
  const values = site || { id: '', name: '', room: '', group: selectedGroup === '*' ? '' : selectedGroup, note: '' };
  const select = form.elements.group;
  select.replaceChildren();
  for (const group of [{ id: '', name: i18n.t('未分組') }, ...state.library.groups]) {
    const option = text('option', group.name); option.value = group.id; select.append(option);
  }
  for (const [key, value] of Object.entries(values)) if (form.elements[key]) form.elements[key].value = value;
  $('#site-dialog-title').textContent = site ? i18n.t('編輯站台') : i18n.t('新增站台');
  openDialog('#site-dialog');
}
function editGroup(group) {
  if (!state) return;
  $('#group-form').reset();
  $('#group-form').elements.id.value = group?.id || '';
  $('#group-form').elements.name.value = group?.name || '';
  $('#group-dialog-title').textContent = group ? i18n.t('編輯群組') : i18n.t('新增群組');
  openDialog('#group-dialog');
}
function confirmDelete(title, description, callback) {
  $('#confirm-form button[type="submit"]').textContent = i18n.t('確認刪除');
  $('#confirm-title').textContent = title;
  $('#confirm-description').textContent = description;
  confirmAction = callback;
  openDialog('#confirm-dialog');
}
function deleteSite(site) {
  confirmDelete(i18n.t('刪除站台'), i18n.t(`確定刪除「${site.name}」？這只會移除本機保存的站台資料。`), async () => {
    const library = structuredClone(state.library);
    library.sites = library.sites.filter(value => value.id !== site.id);
    await save(library);
    toast(i18n.t('站台已刪除'));
  });
}
function deleteGroup(group) {
  confirmDelete(i18n.t('刪除群組'), i18n.t(`確定刪除「${group.name}」？群組中的站台會移到「未分組」。`), async () => {
    const library = structuredClone(state.library);
    library.groups = library.groups.filter(value => value.id !== group.id);
    library.sites.forEach(site => { if (site.group === group.id) site.group = ''; });
    await save(library);
    if (selectedGroup === group.id) { rememberGroup('*'); renderLibrary(); }
    toast(i18n.t('群組已刪除，站台已保留'));
  });
}
// 記住密碼、手動密碼與快速連線共用等待流程。
async function startConnection(key, name, request) {
 document.querySelectorAll('#connect-dialog[open], #quick-password-dialog[open]').forEach(dialog=>dialog.close());
 connectionWait={key,name,accepted:false,failed:false};
 const dialog=$('#connection-progress-dialog');
 dialog.dataset.failed='false';
 $('#connection-progress-title').textContent=i18n.t('正在建立連線');
 $('#connection-progress-name').textContent=name;
 $('#connection-progress-status').textContent=i18n.t('等待遠端回應…');
 $('#connection-progress-cancel').textContent=i18n.t('取消連線');
 openDialog('#connection-progress-dialog');
 try { await request(); connectionWait.accepted=true; await updateRunning(); }
 catch(error) { failConnection(error.message); }
}
function failConnection(message) {
 pendingTerminalSite=null;
 pendingDiagnosticSite=null;
 if(!connectionWait)return;
 connectionWait.failed=true;
 const dialog=$('#connection-progress-dialog');dialog.dataset.failed='true';
 $('#connection-progress-title').textContent=i18n.t('連線未完成');
 $('#connection-progress-status').textContent=i18n.t(message);
 $('#connection-progress-cancel').textContent=i18n.t('關閉');
}
function renderConnectionWait() {
 if(!connectionWait||connectionWait.failed||!connectionWait.accepted)return;
 const {key}=connectionWait, session=state.sessions?.[key];
 const dialog=$('#connection-progress-dialog');
 if(state.passwordPrompt?.key===key){if(dialog.open)dialog.close();return;}
 if(session?.stage==='connected'){dialog.close();connectionWait=null;if(pendingTerminalSite && key===`viewer:${pendingTerminalSite.id}`){const site=pendingTerminalSite;pendingTerminalSite=null;openTerminal(site);return;}if(pendingDiagnosticSite && key===`viewer:${pendingDiagnosticSite.id}`){const site=pendingDiagnosticSite;pendingDiagnosticSite=null;openDiagnostics(site);}return;}
 if(session?.error){failConnection(session.error);return;}
 if(!Object.hasOwn(state.running,key)){failConnection(state.notice||'遠端連線已結束，請確認網路與遠端裝置後重試。');return;}
 const labels={waiting:'等待遠端回應…',authenticating:'正在驗證密碼…',connecting:pendingTerminalSite?'正在準備命令列…':'正在啟動 遠端顯示，等待遠端畫面…'};
 $('#connection-progress-status').textContent=i18n.t(labels[session?.stage]||'正在建立安全連線…');
}
function cancelConnectionWait(){
 pendingTerminalSite=null;
 pendingDiagnosticSite=null;
 if(!connectionWait)return;
 if(connectionWait.failed){$('#connection-progress-dialog').close();connectionWait=null;return;}
 action(async()=>{await api('stop','POST',{key:connectionWait.key});$('#connection-progress-dialog').close();connectionWait=null;await updateRunning();});
}
$('#connection-progress-dialog form').addEventListener('submit',event=>event.preventDefault());
$('#connection-progress-cancel').addEventListener('click',cancelConnectionWait);
$('#connection-progress-dialog').addEventListener('cancel',event=>{event.preventDefault();if(!busy)cancelConnectionWait();});

function connect(site,terminal=false) {
 if(siteRecovery.has(site.id)){toast(i18n.t('等待遠端恢復連線…'));return;}
 if(siteCapability(site,terminal?'terminal':'desktop')===false){toast(i18n.t(terminal?'對方尚未支援命令列，請更新對方的 YourDesk 後再試。':'此裝置沒有桌面環境，請改用命令列連線。'),true);return;}
  action(async () => {
    pendingTerminalSite=terminal?site:null;
    if(terminal)pendingDiagnosticSite=null;
    const saved = await api('remembered', 'POST', { id: site.id });
    if (saved.remembered) { await startConnection(`viewer:${site.id}`,site.name,()=>api('viewer/start','POST',{id:site.id,terminal,diagnostics:pendingDiagnosticSite?.id===site.id})); return; }
    $('#connect-form').reset();
    $('#connect-form').elements.id.value = site.id;
    $('#connect-name').textContent = site.name;
    $('#connect-room').textContent = site.room;
    openDialog('#connect-dialog');
  });
}

async function updateRunning() {
  const latest = await api('state');
  state.hardwareDetection = latest.hardwareDetection;
  if(!$('#settings-panel-hardware').hidden||deepHardwareState?.status==='running')deepHardwareState=await api('hardware-deep');
  renderHardwareAnalysis();
  state.passwordPrompt = latest.passwordPrompt;
  renderStartupOptimization();
  const now=Date.now();
  for(const site of state.library.sites){
   const key=`viewer:${site.id}`;
   if(state.sessions?.[key]?.stage==='connected' && latest.sessions?.[key]?.stage!=='connected'){
    siteRecovery.set(site.id,{readyAfter:now+3000,expires:now+15000});
    setTimeout(refreshPresence,3100);
   }
  }
  for(const [id,recovery] of siteRecovery){
   if(now>=recovery.expires)siteRecovery.delete(id);
  }

  const changed = JSON.stringify(state.running) !== JSON.stringify(latest.running);
  const codecsChanged = JSON.stringify(state.videoCapabilities) !== JSON.stringify(latest.videoCapabilities) || state.hardwareJPEG !== latest.hardwareJPEG;
  const srChanged=JSON.stringify(state.superResolutionCapabilities)!==JSON.stringify(latest.superResolutionCapabilities);
  const modelsChanged=JSON.stringify(state.coreMLModels)!==JSON.stringify(latest.coreMLModels);
  state.coreMLModels=latest.coreMLModels;
  if(modelsChanged)refreshCoreMLModels($('#ui-coreml-model').value||state.preferences?.coreMLModel);
  state.superResolutionCapabilities=latest.superResolutionCapabilities;
  if(srChanged)refreshSuperResolution($('#ui-super-resolution').value||state.preferences?.superResolution);
  state.videoCapabilities = latest.videoCapabilities;
  state.hardwareJPEG = latest.hardwareJPEG;
  if (codecsChanged) refreshCodecOptions($('#stream-codec').value || state.preferences?.codec);
  state.incomingConnected=latest.incomingConnected;
 state.prelogin=latest.prelogin;
 renderPrelogin();
 state.running = latest.running;
  state.quick = latest.quick;
  state.sessions = latest.sessions;
  state.notice = latest.notice;
  showHostConflict(latest.hostConflict);
  state.passwordPrompt = latest.passwordPrompt;
 state.updates = latest.updates;
 renderRelease(latest.updates);
  renderConnectionWait();
  state.info = latest.info;
  if (!connectionWait && latest.notice && latest.notice !== lastNotice) toast(latest.notice, false, { warning: true, duration: 15000 });
  lastNotice = latest.notice;
  renderDevice();
  if (changed) renderSites();
  document.querySelectorAll('.site-card').forEach(card=>applySiteCapabilities(card,state.library.sites.find(site=>site.id===card.dataset.siteId)));
  renderQuick();
}
function stop(key) {
  action(async () => {
    await api('stop', 'POST', { key });
    delete state.running[key];
    renderDevice(); renderSites();
    toast(i18n.t('已送出停止指令'));
  });
}
$('#copy-room').addEventListener('click', () => action(async () => { if (!state) return; await navigator.clipboard.writeText(state.info.room); toast(i18n.t('本機 ID 已複製')); }));
$('#toggle-secret').addEventListener('click', () => {
  const visible = $('#local-secret').type === 'password';
  $('#local-secret').type = visible ? 'text' : 'password';
  $('#toggle-secret').textContent = visible ? i18n.t('隱藏') : i18n.t('顯示');
  $('#toggle-secret').setAttribute('aria-label', visible ? i18n.t('隱藏連線密碼') : i18n.t('顯示連線密碼'));
});
let deepHardwareState=null;
let deepHardwareSubmitting=false;
let hardwareHold=null;
const hardwareDeepButton=$('#hardware-deep-test');
function cancelHardwareHold(){clearTimeout(hardwareHold);hardwareHold=null;hardwareDeepButton.classList.remove('holding');}
function startHardwareHold(){
 if(hardwareHold!==null||hardwareDeepButton.disabled)return;
 hardwareDeepButton.classList.add('holding');
 hardwareHold=setTimeout(()=>{
  cancelHardwareHold();
  if($('#settings-panel-hardware').hidden||!$('#settings-dialog').open||document.hidden)return;
  deepHardwareSubmitting=true;renderHardwareAnalysis();
  action(async()=>{try{deepHardwareState=await api('hardware-deep','POST');}finally{deepHardwareSubmitting=false;renderHardwareAnalysis();}});
 },1000);
}
hardwareDeepButton.addEventListener('pointerdown',event=>{if(event.button===0){event.preventDefault();startHardwareHold();}});
for(const event of ['pointerup','pointerleave','pointercancel','blur'])hardwareDeepButton.addEventListener(event,cancelHardwareHold);
hardwareDeepButton.addEventListener('keydown',event=>{if([' ','Enter'].includes(event.key)){event.preventDefault();if(!event.repeat)startHardwareHold();}});
hardwareDeepButton.addEventListener('keyup',event=>{if([' ','Enter'].includes(event.key)){event.preventDefault();cancelHardwareHold();}});
hardwareDeepButton.addEventListener('click',event=>event.preventDefault());
hardwareDeepButton.addEventListener('contextmenu',event=>event.preventDefault());
window.addEventListener('blur',cancelHardwareHold);
document.addEventListener('visibilitychange',cancelHardwareHold);
$('#settings-dialog').addEventListener('close',cancelHardwareHold);
function renderHardwareAnalysis() {
 const panel=$('#settings-panel-hardware');
 if(panel.hidden)return;
 const detection=deepHardwareState&&deepHardwareState.status!=='not-started'?deepHardwareState:state?.hardwareDetection;
 const busy=deepHardwareSubmitting||deepHardwareState?.status==='running';
 hardwareDeepButton.disabled=busy||!state?.hardwareDetection||['running','not-started'].includes(state.hardwareDetection.status);
 hardwareDeepButton.setAttribute('aria-busy',String(busy));
 const progress=$('#hardware-deep-progress');
 progress.hidden=!busy;
 const progressLabel=$('#hardware-deep-progress-label');
 progressLabel.hidden=progress.hidden;
 if(deepHardwareSubmitting){
  progress.removeAttribute('value');
  progressLabel.textContent=i18n.t('正在啟動深度測試…');
 }else{
  const total=Math.max(1,deepHardwareState?.total||0);
  const completed=deepHardwareState?.completed||0;
  progress.max=total;progress.value=completed;
  progressLabel.textContent=`${i18n.t('深度測試')} · ${Math.round(completed/total*100)}% · ${completed}/${deepHardwareState?.total||0}`;
 }
 const t=value=>i18n.t(value);
 const unknown=t('未確認');
 const states={'not-started':'尚未開始偵測',running:'偵測中',complete:'偵測完成',partial:'部分偵測未完成',failed:'偵測未完成',cancelled:'偵測已取消',pending:'等待偵測'};
 const status=t(states[detection?.status]||'尚未開始偵測');
 const elapsed=Number.isFinite(detection?.durationMS)?` · ${Math.round(detection.durationMS)} ms`:'';
 $('#hardware-analysis-status').hidden=!busy&&detection?.status!=='running';
 $('#hardware-analysis-status').textContent=`${status}${detection?` · ${detection.completed}/${detection.total}`:''}${elapsed}`;
 const root=$('#hardware-analysis-results');
 // 狀態輪詢不重建未變動的內容，保留使用者選取與捲動位置。
 const signature=JSON.stringify([detection,i18n.t('編解碼分析'),state?.info]);
 if(root.dataset.signature===signature)return;
 root.dataset.signature=signature;root.replaceChildren();
 const heading=label=>root.append(text('h4',t(label)));
 const list=()=>{const node=document.createElement('dl');node.className='settings-details';root.append(node);return node;};
 const row=(node,label,value)=>{const item=document.createElement('div');item.append(text('dt',t(label)),text('dd',String(value??unknown)));node.append(item);};
 const results=detection?.results||[];
 const inventory=results.find(r=>r.key==='inventory'&&r.state==='complete')?.data||{};
 heading('硬體概況');const overview=list();
 row(overview,'CPU',inventory.cpu?.brand||unknown);
 row(overview,'邏輯處理器',inventory.logicalCPUs??inventory.cpu?.['hw.logicalcpu']);
 row(overview,'GPU',inventory.gpus?.map(g=>g.name).filter(Boolean).join(' / ')||unknown);
 const memory=inventory.memoryBytes??inventory.cpu?.['hw.memsize'];
 row(overview,'記憶體',Number.isFinite(memory)&&memory>0?`${Number((memory/1024**3).toFixed(1))} GiB`:unknown);
 if(typeof inventory.preferredGraphicsAPI==='string'){
  const api=inventory.preferredGraphicsAPI, level=inventory.preferredGraphicsFeatureLevel;
  row(overview,'優先圖形 API',api?`${api}${level?` (FL ${level})`:''}`:t(inventory.graphicsDetectionIncomplete?'偵測未完成':'不支援'));
 }
 for(const gpu of inventory.gpus||[]){
  for(const apiName of ['d3d11','d3d12']){
   if(!(apiName in gpu))continue;
   const level=gpu[`${apiName}FeatureLevel`];
   row(overview,`${gpu.name} · ${apiName.toUpperCase()}`,gpu[apiName]?`${t('可用')}${level?` (FL ${level})`:''}`:t(gpu[apiName]===false?'不支援':'偵測未完成'));
  }
 }
 heading('加速指令集');const features=list();
 const flags=inventory.cpuFeatures||Object.fromEntries(Object.entries({'NEON / ASIMD':'hw.optional.neon',DotProd:'hw.optional.arm.FEAT_DotProd',I8MM:'hw.optional.arm.FEAT_I8MM',SVE:'hw.optional.arm.FEAT_SVE',SME:'hw.optional.arm.FEAT_SME',SSE2:'hw.optional.sse2',SSSE3:'hw.optional.supplementalsse3',AVX2:'hw.optional.avx2_0',AVX512F:'hw.optional.avx512f'}).map(([name,key])=>[name,inventory.cpu?.[key]]));
 // 指令集缺少可用證據時，介面保守顯示不支援。
 for(const [name,value] of Object.entries(flags))row(features,name,value===true||value===1?t('可用'):value===false||value===0?t('未提供'):t('不支援'));
 if(!Object.keys(flags).length)row(features,'偵測結果',t('不支援'));
 root.append(text('p',t('結果僅代表列出的格式與尺寸；實測未通過或未確認的項目，保守顯示為不支援。'),'help-description'));
 // 原生 NSNumber 與 Go JSON 的布林表示不同；缺值仍保留未知。
 const probeBool=value=>value===true||value===1?true:value===false||value===0?false:undefined;
 const outcome=(data,decode)=>{
  const statusKeys=decode?['decoder_create_status','decode_status','decode_callback_status']:['create_status','prepare_status','buffer_status','buffer_lock_status','encode_status','encode_callback_status'];
  if(statusKeys.some(key=>Number.isFinite(data[key])&&data[key]!==0))return t('不支援');
  if(!decode&&data.status==='unavailable')return t('不支援');
  const ok=probeBool(decode?(data.decodeOK??data.decoded_size_matches):(data.encodeOK??data.sample_produced));
  if(ok===false)return t('不支援');
  if(ok!==true)return t('不支援');
  const hardware=probeBool(decode?(data.hardwareDecoder??data.hardware_decoder?.value):(data.hardwareEncoder??data.hardware_encoder?.value));
  const mode=decode?data.decodingMode:null;
  return t(hardware===true||mode==='hardware'?'硬體加速可用':hardware===false||mode==='software'?'軟體運算':'可用 (加速未知)');
 };
 const separated=results.some(r=>/\/(encode|decode)$/.test(r.key));
 for(const decode of [false,true]){
  heading(decode?'解碼實測':'編碼實測');
  const wrap=document.createElement('div');wrap.className='hardware-table-wrap';
  const table=document.createElement('table');table.className='hardware-table';
  const head=table.createTHead().insertRow();
  for(const label of [decode?'格式 / 輸出':'格式 / 輸入','結果','後端','耗時']){const th=text('th',t(label));th.scope='col';head.append(th);}
  const body=table.createTBody();
  for(const result of results.filter(r=>r.key!=='inventory'&&!(r.data?.width===128&&r.data?.height===128)&&!/(?:^|\/)128x128(?:\/|$)/.test(r.key)&&(!separated||r.key.endsWith(decode?'/decode':'/encode')))){
   const data=result.data||{};const tr=body.insertRow();const base=result.key.replace(/\/(encode|decode)$/,'').replace(/\/1920x1080$/,'');
   tr.append(text('td',(data.probeKind==='windows-software'||data.probeKind==='software')?`${data.codec} / ${t('軟體獨立測試')}${data.width===1920&&data.height===1080?'':` / ${data.width}×${data.height}`}`:data.probeKind==='windows-native'?`${t('Windows 原生格式')} / ${base}`:base));
   tr.append(text('td',result.state==='complete'?outcome(data,decode):t(states[result.state]||'偵測未完成')));
   tr.append(text('td',(decode?data.decoderBackend:(data.encoderBackend||data.backend))||'—'));
   tr.append(text('td',separated&&Number.isFinite(result.durationMS)&&result.state!=='pending'?`${Math.round(result.durationMS)} ms`:'—'));
  }
  wrap.append(table);root.append(wrap);
 }

}
function selectSettingsTab(name) {
  cancelHardwareHold();
  document.querySelectorAll('[data-settings-tab]').forEach(tab => {
    const active = tab.dataset.settingsTab === name;
    tab.setAttribute('aria-selected', String(active));
    tab.tabIndex = active ? 0 : -1;
    document.getElementById(tab.getAttribute('aria-controls')).hidden = !active;
  });
  // 切換功能頁時重新遮蔽密碼。
  if(name==='hardware')renderHardwareAnalysis();
  $('#local-secret').type = 'password';
  $('#toggle-secret').textContent = i18n.t('顯示');
  $('#toggle-secret').setAttribute('aria-label', i18n.t('顯示連線密碼'));
}
const settingsTabs = [...document.querySelectorAll('[data-settings-tab]')];
settingsTabs.forEach((tab, index) => {
  tab.addEventListener('click', () => selectSettingsTab(tab.dataset.settingsTab));
  tab.addEventListener('keydown', event => {
    let next = index;
    if (event.key === 'ArrowDown') next = (index + 1) % settingsTabs.length;
    else if (event.key === 'ArrowUp') next = (index + settingsTabs.length - 1) % settingsTabs.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = settingsTabs.length - 1;
    else return;
    event.preventDefault();
    selectSettingsTab(settingsTabs[next].dataset.settingsTab);
    settingsTabs[next].focus();
  });
});
$('#open-settings').addEventListener('click', () => {
  if (!state) { toast(i18n.t('本機資訊尚未載入，請稍後再試。'), true); return; }
  const { info } = state;
  $('#settings-hostname').textContent = info.hostname || i18n.t('本機電腦');
  $('#settings-platform').textContent = `${{ darwin: 'macOS', windows: 'Windows', linux: 'Linux' }[info.platform] || info.platform} / ${info.architecture}`;
  $('#settings-room').textContent = info.room;
  $('#settings-version').textContent = info.version;
  $('#settings-path').textContent = info.configPath;
  $('#settings-path').dataset.tooltip = info.configPath;
  selectSettingsTab('general');
  openDialog('#settings-dialog');
});
$('#add-site').addEventListener('click', () => editSite());
$('#search').addEventListener('input', () => { if (state) renderSites(); });
$('.brand').addEventListener('click', event => { event.preventDefault(); rememberGroup('*'); $('#search').value = ''; if (state) renderLibrary(); });
document.querySelectorAll('dialog .close').forEach(element => element.addEventListener('click', () => { if (!busy) element.closest('dialog').close(); }));
document.querySelectorAll('dialog').forEach(dialog => dialog.addEventListener('cancel', event => { if (busy) event.preventDefault(); }));
$('#connect-dialog').addEventListener('close', () => { $('#connect-form').elements.secret.value = ''; });
$('#site-form').addEventListener('submit', event => {
  event.preventDefault();
  action(async () => {
    const values = Object.fromEntries(new FormData(event.target));
    for (const key of Object.keys(values)) values[key] = values[key].trim();
    values.id ||= crypto.randomUUID();
    const library = structuredClone(state.library);
    const index = library.sites.findIndex(site => site.id === values.id);
    if (index === -1) library.sites.push(values); else library.sites[index] = values;
    await save(library);
    $('#site-dialog').close(); toast(i18n.t('站台已儲存'));
  });
});
$('#group-form').addEventListener('submit', event => {
  event.preventDefault();
  action(async () => {
    const group = Object.fromEntries(new FormData(event.target));
    group.name = group.name.trim();
    group.id ||= crypto.randomUUID();
    const library = structuredClone(state.library);
    const index = library.groups.findIndex(value => value.id === group.id);
    if (index === -1) library.groups.push(group); else library.groups[index] = group;
    await save(library);
    $('#group-dialog').close(); toast(i18n.t('群組已儲存'));
  });
});
$('#connect-form').addEventListener('submit', event => {
  event.preventDefault();
  action(async () => {
    const form = event.target;
    const payload={id:form.elements.id.value,secret:form.elements.secret.value,remember:form.elements.remember.checked};
    payload.terminal=pendingTerminalSite?.id===payload.id;
    payload.diagnostics=pendingDiagnosticSite?.id===payload.id;
    await startConnection(`viewer:${payload.id}`,$('#connect-name').textContent,()=>api('viewer/start','POST',payload));
  });
});
$('#confirm-form').addEventListener('submit', event => {
  event.preventDefault();
  action(async () => { if (confirmAction) await confirmAction(); $('#confirm-dialog').close(); confirmAction = null; });
});
function renderQuick() {
  const quick = state.quick;
  $('#quick-room').disabled = !!quick;
  $('#quick-submit').disabled = !!quick;
  $('#quick-stop').hidden = !quick;
  $('#quick-status').hidden = !quick;
  if (quick) {
    const labels = { waiting: i18n.t('等待遠端回應'), password: i18n.t('等待輸入連線密碼'), authenticating: i18n.t('正在驗證密碼'), connected: i18n.t('已收到遠端畫面') };
    $('#quick-status').textContent = `${quick.room} · ${labels[quick.stage] || i18n.t('連線中')}`;
    $('#quick-stop').dataset.tooltip = quick.stage === 'connected' ? i18n.t('關閉快速連線') : i18n.t('取消快速連線');
  }
  const prompt = state.passwordPrompt;
  const dialog = $('#quick-password-dialog');
  if (!prompt) { if (dialog.open) dialog.close(); return; }
  $('#quick-password-room').textContent = prompt.room;
  $('#quick-password-message').textContent = i18n.t(prompt.message);
  if (!dialog.open && !document.querySelector('dialog[open]')) {
    $('#quick-password-form').reset();
    $('#quick-password-form').elements.remember.checked = !!prompt.remember;
    dialog.dataset.processKey = prompt.key;
    openDialog('#quick-password-dialog');
  }
}
$('#quick-form').addEventListener('submit', event => {
	event.preventDefault();
	action(async () => {
		const room = $('#quick-room').value.trim();
		if (!room) throw new Error(i18n.t('請先貼上遠端 ID'));
		$('#quick-password-form').reset();
		$('#quick-password-room').textContent = room;
		$('#quick-password-message').textContent = i18n.t('此裝置需要連線密碼，請輸入被控端提供的密碼。');
		$('#quick-password-dialog').dataset.processKey = 'new';
		openDialog('#quick-password-dialog');
	});
});
function cancelQuick(key = 'quick') {
	if (key === 'new') {
		$('#quick-password-dialog').close();
		return;
	}
	action(async () => {
    await api('stop', 'POST', { key });
    $('#quick-password-dialog').close();
    if(connectionWait?.key===key)connectionWait=null;
    if (key === 'quick') state.quick = null;
    state.passwordPrompt = null;
    renderQuick();
    toast(i18n.t('快速連線已關閉'));
  });
}
$('#quick-stop').addEventListener('click', () => cancelQuick());
document.querySelectorAll('.quick-cancel').forEach(element => element.addEventListener('click', () => cancelQuick($('#quick-password-dialog').dataset.processKey)));
$('#quick-password-dialog').addEventListener('cancel', event => { event.preventDefault(); if (!busy) cancelQuick($('#quick-password-dialog').dataset.processKey); });
$('#quick-password-dialog').addEventListener('close', () => { $('#quick-password-form').elements.secret.value = ''; });
$('#quick-password-form').addEventListener('submit', event => {
	event.preventDefault();
	action(async () => {
		const key = $('#quick-password-dialog').dataset.processKey;
    const payload={secret:event.target.elements.secret.value,remember:event.target.elements.remember.checked};
    const room=$('#quick-room').value.trim();
    await startConnection(key==='new'?'quick':key,$('#quick-password-room').textContent,()=>key==='new'?api('quick/start','POST',{...payload,room}):api('quick/password','POST',{...payload,key}));
  });
});
$('#settings-dialog').addEventListener('close', () => {
  $('#local-secret').type = 'password';
  $('#toggle-secret').textContent = i18n.t('顯示');
  $('#toggle-secret').setAttribute('aria-label', i18n.t('顯示連線密碼'));
});
const systemTheme = matchMedia('(prefers-color-scheme: dark)');
function applyPreferences(preferences) {
  $('#ui-mcp-enabled').checked=!!preferences?.mcpEnabled;
 $('#ui-mcp-open-display').checked=!!preferences?.mcpOpenDisplay;
 $('#ui-mcp-whitelist-enabled').checked=preferences?.mcpWhitelistEnabled!==false;
 $('#ui-mcp-whitelist').value=(preferences?.mcpWhitelist || ['127.0.0.1']).join('\n');
 $('#ui-mcp-whitelist').disabled=!$('#ui-mcp-whitelist-enabled').checked;
  const values = preferences || { language: 'auto', theme: 'default' };
  i18n.apply(values.language);
  document.documentElement.dataset.theme = values.theme === 'default' ? (systemTheme.matches ? 'dark' : 'light') : values.theme;
  $('#ui-language').value = values.language;
  $('#ui-theme').value = values.theme;
  $('#ui-hints').checked = !values.disableHints;
 $('#ui-key-mapping').checked=!values.disableKeyMapping;
 $('#ui-fit-window').checked=!!values.fitWindow;
 $('#ui-close-on-disconnect').checked=!!values.closeWindowOnDisconnect;
 $('#ui-enhancement').checked=!!values.imageEnhancement;
 $('#ui-interpolation').checked=!!values.interpolation;
 $('#ui-interpolation-method').value=values.interpolationMethod||'';
 $('#ui-gop').value=values.keyframeInterval||10;
 $('#ui-source-fps').value=(values.sourceFPSLimit>=5&&values.sourceFPSLimit<=60?values.sourceFPSLimit:20);$('#ui-bitrate-limit').value=(values.bitrateLimitMbps>=1&&values.bitrateLimitMbps<=64?values.bitrateLimitMbps:12);
 renderInterpolationSupport();
 refreshSuperResolution(values.superResolution);
 refreshCoreMLModels(values.coreMLModel);
 $('#ui-enhancement-strategy').value=values.enhancementStrategy||'quality';
 $('#ui-enhancement-budget').value=values.enhancementBitrateMbps||12;
 renderEnhancementStrategy();
	$('#direct-listen').checked = !!values.directListen;
 $('#tailcat-mode').checked = !!values.tailcatEnabled;
 $('#transport-summary').textContent=i18n.t(values.tailcatEnabled?'WebRTC / Tailcat 虛擬傳輸':'WebRTC / 自動協商');
 renderPrelogin();
  hints.setEnabled(!values.disableHints);
  refreshCodecOptions(values.codec);
 $('#stream-codec-goal').value=values.codecGoal||'balanced';
  if (state) { renderDevice(); renderLibrary(); renderQuick();
 renderRelease(state.updates); }
}
function refreshCodecGoalState() {
 const codecs=$('#stream-codec');
 $('#stream-codec-goal').disabled=codecs.disabled||codecs.value!=='auto';
}
function refreshCodecOptions(selected) {
  const codecs = $('#stream-codec');
  codecs.replaceChildren();
  const available = document.createElement('optgroup'); available.label = i18n.t('可用');
  const unsupported = document.createElement('optgroup'); unsupported.label = i18n.t('不支援'); unsupported.disabled = true;
  for (const [value, label, supported] of [
    ['auto', '自動 (按排列順序優先)', true],
    ['hardware-hevc', 'HEVC (H.265) 硬體', !!state?.videoCapabilities?.some(cap => cap.codec === 'hardware-hevc' && cap.encode)],
    ['hardware-h264', 'H.264 硬體', !!state?.videoCapabilities?.some(cap => cap.codec === 'hardware-h264' && cap.encode)],
    ['software-av1', 'AV1 軟體', !!state?.videoCapabilities?.some(cap => cap.codec === 'software-av1' && cap.encode)],
    ['hardware-av1', 'AV1 硬體', !!state?.videoCapabilities?.some(cap => cap.codec === 'hardware-av1' && cap.encode)],
    ['hardware-jpeg', 'JPEG 硬體', !!state?.hardwareJPEG],
    ['software-jpeg', 'JPEG 軟體', true]
  ]) {
    if (value === 'hardware-av1' && !state?.videoCapabilities?.some(cap => cap.codec === 'hardware-av1')) continue;
    const option = text('option', i18n.t(label)); option.value = value; option.disabled = !supported;
    (supported ? available : unsupported).append(option);
  }
  codecs.append(available, unsupported); codecs.value = selected || 'auto';
 refreshCodecGoalState();

}
async function savePreferences() {
  if (!state || busy) return;
 for(const id of ['#ui-source-fps','#ui-bitrate-limit','#ui-gop']){if(!$(id).checkValidity()){$(id).reportValidity();return}}
  const previous = state.preferences;
 if(!$('#ui-enhancement-budget').checkValidity()){$('#ui-enhancement-budget').reportValidity();return}
	const preferences = { tailcatEnabled:$('#tailcat-mode').checked, mcpOpenDisplay:$('#ui-mcp-open-display').checked, mcpWhitelistEnabled:$('#ui-mcp-whitelist-enabled').checked, mcpWhitelist:[...new Set($('#ui-mcp-whitelist').value.split(/\r?\n/).map(v=>v.trim()).filter(Boolean))], mcpEnabled:$('#ui-mcp-enabled').checked, fitWindow:$('#ui-fit-window').checked, closeWindowOnDisconnect:$('#ui-close-on-disconnect').checked, sourceFPSLimit:Number($('#ui-source-fps').value), bitrateLimitMbps:Number($('#ui-bitrate-limit').value), keyframeInterval:Number($('#ui-gop').value), interpolation: $('#ui-interpolation').checked, interpolationMethod: $('#ui-interpolation-method').value, coreMLModel: $('#ui-coreml-model').value || 'quicksrnet-small', enhancementStrategy: $('#ui-enhancement-strategy').value, enhancementBitrateMbps: Number($('#ui-enhancement-budget').value), superResolution: $('#ui-super-resolution').value, imageEnhancement: $('#ui-enhancement').checked, language: $('#ui-language').value, theme: $('#ui-theme').value, codec: $('#stream-codec').value, codecGoal: $('#stream-codec-goal').value, disableHints: !$('#ui-hints').checked, disableKeyMapping:!$('#ui-key-mapping').checked, directListen: $('#direct-listen').checked };
  $('#ui-mcp-whitelist-enabled').disabled=true;$('#ui-mcp-whitelist').disabled=true;
 $('#ui-mcp-enabled').disabled=true;
 $('#ui-language').disabled = true;
  $('#ui-theme').disabled = true;
  $('#ui-hints').disabled = true;
 $('#ui-key-mapping').disabled=true;
 $('#ui-fit-window').disabled=true;
 $('#ui-source-fps').disabled=true;$('#ui-bitrate-limit').disabled=true;$('#ui-gop').disabled=true;$('#ui-interpolation').disabled=true;$('#ui-interpolation-method').disabled=true;$('#ui-enhancement').disabled=true;$('#ui-super-resolution').disabled=true;$('#ui-coreml-model').disabled=true;$('#ui-enhancement-strategy').disabled=true;$('#ui-enhancement-budget').disabled=true;
  $('#stream-codec').disabled = true;
 $('#stream-codec-goal').disabled = true;
	$('#direct-listen').disabled = true;
 $('#tailcat-mode').disabled = true;
  await action(async () => {
    try {
      await api('preferences', 'PUT', preferences);
      state.preferences = preferences;
      applyPreferences(preferences);
    } catch (error) { applyPreferences(previous); throw error; }
  });
  $('#ui-mcp-whitelist-enabled').disabled=false;$('#ui-mcp-whitelist').disabled=!$('#ui-mcp-whitelist-enabled').checked;
 $('#ui-mcp-enabled').disabled=false;
 $('#ui-language').disabled = false;
  $('#ui-theme').disabled = false;
  $('#ui-hints').disabled = false;
 $('#ui-key-mapping').disabled=false;
 $('#ui-fit-window').disabled=false;
 $('#ui-source-fps').disabled=false;$('#ui-bitrate-limit').disabled=false;$('#ui-gop').disabled=false;$('#ui-interpolation').disabled=false;renderInterpolationSupport();$('#ui-enhancement').disabled=false;$('#ui-super-resolution').disabled=false;$('#ui-coreml-model').disabled=false;$('#ui-enhancement-strategy').disabled=false;$('#ui-enhancement-budget').disabled=false;
  $('#stream-codec').disabled = false;
 refreshCodecGoalState();
	$('#direct-listen').disabled = false;
 $('#tailcat-mode').disabled = false;
}
$('#ui-mcp-whitelist-enabled').addEventListener('change',savePreferences);
$('#ui-mcp-whitelist').addEventListener('change',savePreferences);
$('#ui-mcp-enabled').addEventListener('change',savePreferences);
$('#ui-mcp-open-display').addEventListener('change',savePreferences);
$('#ui-language').addEventListener('change', savePreferences);
$('#ui-theme').addEventListener('change', savePreferences);
$('#ui-hints').addEventListener('change', savePreferences);
$('#ui-key-mapping').addEventListener('change',savePreferences);
$('#ui-fit-window').addEventListener('change',savePreferences);
$('#ui-close-on-disconnect').addEventListener('change',savePreferences);
 $('#ui-enhancement').addEventListener('change',savePreferences);
 $('#ui-interpolation').addEventListener('change',savePreferences);
 $('#ui-interpolation-method').addEventListener('change',savePreferences);
 $('#ui-gop').addEventListener('change',savePreferences);
 $('#ui-source-fps').addEventListener('change',savePreferences);$('#ui-bitrate-limit').addEventListener('change',savePreferences);
 $('#ui-super-resolution').addEventListener('change',()=>{renderCoreMLModelRow();savePreferences();});
 $('#ui-coreml-model').addEventListener('change',savePreferences);
 $('#ui-enhancement-strategy').addEventListener('change',()=>{renderEnhancementStrategy();savePreferences()});
 $('#ui-enhancement-budget').addEventListener('change',()=>{renderEnhancementStrategy();savePreferences()});
$('#stream-codec').addEventListener('change', () => { refreshCodecGoalState(); savePreferences(); });
$('#stream-codec-goal').addEventListener('change', savePreferences);
$('#direct-listen').addEventListener('change', savePreferences);
systemTheme.addEventListener('change', () => { if (state) applyPreferences(state.preferences); });
window.addEventListener('languagechange', () => { if (state?.preferences.language === 'auto') applyPreferences(state.preferences); });
async function initialize() {
  try {
    await i18n.load();
    state = await api('state');
    state.library.groups ||= [];
    state.library.sites ||= [];
    selectedGroup=state.preferences?.selectedGroup ?? '*';
    applyPreferences(state.preferences);
    renderDevice(); renderLibrary(); renderQuick();
    // 先讓主畫面完成一次繪製，再決定是否需要顯示背景最佳化提示。
    requestAnimationFrame(() => requestAnimationFrame(() => {
      startupMainReady = true;
      renderStartupOptimization();
    }));
    window.yourdeskInterfaceReady?.();
    refreshPresence();
  } catch (error) {
    $('#connection-error').textContent = i18n.t(`無法載入介面：${error.message}。請重新執行 runUITest.command。`);
    $('#connection-error').hidden = false;
    return;
  }
  setInterval(async () => {
    if (busy || statePolling || (document.hidden && !connectionWait)) return;
    if (!connectionWait && !startupOptimizationPending() && deepHardwareState?.status!=='running' && Date.now()-lastStatePoll<2500) return;
    lastStatePoll=Date.now();
    statePolling=true;
    try {
      await updateRunning();
      $('#connection-error').hidden = true;
    } catch {
      // 本機服務失聯時不要留下無限旋轉的 modal；偵測仍由後端自行管理。
      if ($('#startup-optimization-dialog').open) $('#startup-optimization-dialog').close();
      $('#connection-error').textContent = i18n.t('本機 Client UI 服務無法連線，請確認啟動終端仍在執行。');
      $('#connection-error').hidden = false;
      if(connectionWait)failConnection('本機 Client UI 服務無法連線，請確認啟動終端仍在執行。');
    } finally { statePolling=false; }
  }, 500);
}
initialize();

$('#check-update').addEventListener('click', async () => {
  const control = $('#check-update');
  control.disabled = true;
  $('#download-update').hidden = true;
  $('#update-status').textContent = i18n.t('正在檢查更新…');
  try {
    const result = await api($('#force-update').checked?'updates?force=true':'updates');
    $('#update-status').textContent = i18n.t(result.message) + (result.version ? ` ${result.version}` : '');
    $('#download-update').hidden = !result.available;
    state.updates=result;renderRelease(result,true);
  } catch (error) { $('#update-status').textContent = i18n.t(error.message); }
  finally { control.disabled = false; }
});
$('#download-update').addEventListener('click', () => action(downloadUpdate));

// 使用原生拖曳影像預覽；篩選時僅重排可見站台，隱藏項目保留原位置。
const siteList = $('#site-list');
function clearSiteDrag() {
  siteDrag = null;
  siteList.querySelectorAll('.dragging, .drop-before, .drop-after').forEach(card => card.classList.remove('dragging', 'drop-before', 'drop-after'));
}
siteList.addEventListener('dragstart', event => {
  const card = event.target.closest('.site-card');
  if (!card || busy) { event.preventDefault(); return; }
  siteDrag = { id: card.dataset.siteId, target: null, after: false };
  event.dataTransfer.effectAllowed = 'move';
  event.dataTransfer.setData('text/plain', siteDrag.id);
  const bounds = card.getBoundingClientRect();
  event.dataTransfer.setDragImage(card, event.clientX - bounds.left, event.clientY - bounds.top);
  requestAnimationFrame(() => { if (siteDrag?.id === card.dataset.siteId) card.classList.add('dragging'); });
});
siteList.addEventListener('dragover', event => {
  if (!siteDrag) return;
  event.preventDefault();
  event.dataTransfer.dropEffect = 'move';
  siteList.querySelectorAll('.drop-before, .drop-after').forEach(card => card.classList.remove('drop-before', 'drop-after'));
  const cards = [...siteList.querySelectorAll('.site-card')];
  const target = cards.find(card => event.clientY < card.getBoundingClientRect().bottom) || cards.at(-1);
  siteDrag.target = target?.dataset.siteId || null;
  if (target && siteDrag.target !== siteDrag.id) {
    const bounds = target.getBoundingClientRect();
    siteDrag.after = event.clientY >= bounds.top + bounds.height / 2;
    target.classList.add(siteDrag.after ? 'drop-after' : 'drop-before');
  }
  const bounds = siteList.getBoundingClientRect();
  if (event.clientY < bounds.top + 36) siteList.scrollTop -= 18;
  else if (event.clientY > bounds.bottom - 36) siteList.scrollTop += 18;
});
siteList.addEventListener('dragleave', event => {
  if (siteDrag && !siteList.contains(event.relatedTarget)) {
    siteDrag.target = null;
    siteList.querySelectorAll('.drop-before, .drop-after').forEach(card => card.classList.remove('drop-before', 'drop-after'));
  }
});
siteList.addEventListener('drop', event => {
  if (!siteDrag) return;
  event.preventDefault();
  const { id, target, after } = siteDrag;
  const visible = [...siteList.querySelectorAll('.site-card')].map(card => card.dataset.siteId);
  clearSiteDrag();
  if (!target || id === target || busy) return;
  const ordered = visible.filter(value => value !== id);
  ordered.splice(ordered.indexOf(target) + (after ? 1 : 0), 0, id);
  if (ordered.every((value, index) => value === visible[index])) return;
  action(async () => {
    const library = structuredClone(state.library);
    const byID = new Map(library.sites.map(site => [site.id, site]));
    const visibleIDs = new Set(visible);
    let index = 0;
    library.sites = library.sites.map(site => visibleIDs.has(site.id) ? byID.get(ordered[index++]) : site);
    await save(library);
    toast(i18n.t('站台順序已儲存'));
  });
});
siteList.addEventListener('dragend', () => { clearSiteDrag(); if (state) renderSites(); });

async function refreshPresence() {
  if (!state || checkingPresence || document.hidden) return;
  checkingPresence = true;
  const requestedAt=Date.now();
  try { sitePresence = await api('presence'); }
  catch { sitePresence = {}; }
  finally { checkingPresence = false; }
  for(const [id,recovery] of siteRecovery){
   const site=state.library.sites.find(site=>site.id===id);
   if(Date.now()>=recovery.expires || (requestedAt>=recovery.readyAfter && siteOnline(site)===true))siteRecovery.delete(id);
  }
  // 僅更新圖示，避免干擾拖拉或鍵盤焦點。
  document.querySelectorAll('.site-card').forEach(card => {
    const site=state.library.sites.find(site=>site.id===card.dataset.siteId);
    applySiteCapabilities(card,site);
    const icon = card.querySelector('.mini-device');
    const online = siteOnline(state.library.sites.find(site => site.id === card.dataset.siteId));
    icon.classList.toggle('online', online === true);
    icon.classList.toggle('offline', online !== true);
    icon.dataset.tooltip = i18n.t(online === true ? 'Client 在線上' : online === false ? 'Client 不在線上' : '無法確認 Client 狀態');
  });
}
setInterval(refreshPresence, 5000);
document.addEventListener('visibilitychange', () => { if (!document.hidden) refreshPresence(); });

$('#sidebar-add-group').addEventListener('click', () => editGroup());
const groupMenu = text('div', '', 'group-context-menu');
groupMenu.setAttribute('role', 'menu'); groupMenu.hidden = true;
document.body.append(groupMenu);
let groupMenuAnchor;
function closeGroupMenu(restoreFocus = false) {
  groupMenu.hidden = true;
  if (restoreFocus) groupMenuAnchor?.focus();
}
function openGroupMenu(group, x, y, anchor) {
  if (busy) return;
  groupMenuAnchor = anchor;
  groupMenu.replaceChildren();
  for (const [label, handler, danger] of [['改名', () => editGroup(group), false], ['刪除', () => deleteGroup(group), true]]) {
    const item = button(i18n.t(label), `group-menu-item${danger ? ' danger' : ''}`, () => { closeGroupMenu(); handler(); });
    item.setAttribute('role', 'menuitem'); groupMenu.append(item);
  }
  groupMenu.hidden = false;
  groupMenu.style.left = `${Math.max(8, Math.min(x, innerWidth - groupMenu.offsetWidth - 8))}px`;
  groupMenu.style.top = `${Math.max(8, Math.min(y, innerHeight - groupMenu.offsetHeight - 8))}px`;
  groupMenu.firstElementChild.focus();
}
document.addEventListener('pointerdown', event => { if (!groupMenu.contains(event.target)) closeGroupMenu(); });
window.addEventListener('resize', () => closeGroupMenu());
$('#groups').addEventListener('scroll', () => closeGroupMenu());
groupMenu.addEventListener('keydown', event => {
  if (event.key === 'Escape' || event.key === 'Tab') { closeGroupMenu(true); if (event.key === 'Escape') event.preventDefault(); return; }
  const items = [...groupMenu.children];
  const index = items.indexOf(document.activeElement);
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); items[(index + (event.key === 'ArrowDown' ? 1 : items.length - 1)) % items.length].focus(); }
});
function clearGroupDrag() {
  groupDrag = null;
  $('#groups').querySelectorAll('.dragging, .drop-before, .drop-after').forEach(row => row.classList.remove('dragging', 'drop-before', 'drop-after'));
}
$('#groups').addEventListener('dragstart', event => {
  const row = event.target.closest('[data-group-id]');
  if (!row || busy) { event.preventDefault(); return; }
  closeGroupMenu(); groupDrag = { id: row.dataset.groupId, target: null, after: false };
  event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', groupDrag.id);
  requestAnimationFrame(() => { if (groupDrag?.id === row.dataset.groupId) row.classList.add('dragging'); });
});
$('#groups').addEventListener('dragover', event => {
  if (!groupDrag) return;
  event.preventDefault(); event.dataTransfer.dropEffect = 'move';
  const rows = [...$('#groups').querySelectorAll('[data-group-id]')];
  rows.forEach(row => row.classList.remove('drop-before', 'drop-after'));
  const target = rows.find(row => event.clientY < row.getBoundingClientRect().bottom) || rows.at(-1);
  groupDrag.target = target?.dataset.groupId;
  if (target && groupDrag.target !== groupDrag.id) {
    const bounds = target.getBoundingClientRect(); groupDrag.after = event.clientY >= bounds.top + bounds.height / 2;
    target.classList.add(groupDrag.after ? 'drop-after' : 'drop-before');
  }
  const bounds = $('#groups').getBoundingClientRect();
  if (event.clientY < bounds.top + 25) $('#groups').scrollTop -= 12;
  else if (event.clientY > bounds.bottom - 25) $('#groups').scrollTop += 12;
});
$('#groups').addEventListener('drop', event => {
  if (!groupDrag) return;
  event.preventDefault();
  const { id, target, after } = groupDrag; clearGroupDrag();
  if (!target || id === target || busy) return;
  action(async () => {
    const library = structuredClone(state.library);
    const moved = library.groups.find(group => group.id === id);
    if (!moved) return;
    library.groups = library.groups.filter(group => group.id !== id);
    const index = library.groups.findIndex(group => group.id === target);
    if (index < 0) return;
    library.groups.splice(index + (after ? 1 : 0), 0, moved);
    await save(library); toast(i18n.t('群組順序已儲存'));
  });
});
$('#groups').addEventListener('dragend', clearGroupDrag);

$('#dismiss-password-reminder').addEventListener('click', () => { passwordReminderDismissed = true; $('#password-reminder').hidden = true; });
$('#change-initial-password').addEventListener('click', () => { $('#open-settings').click(); selectSettingsTab('security'); $('#new-local-secret').focus(); });
$('#save-local-password').addEventListener('click', () => action(async () => {
  const input = $('#new-local-secret');
  if (!input.value || !input.reportValidity()) return;
  await api('local-password', 'PUT', { secret: input.value });
  input.value = '';
  await updateRunning();
  toast(i18n.t('連線密碼已更新'));
}));
$('#settings-dialog').addEventListener('close', () => { $('#new-local-secret').value = ''; });

$('#export-config').addEventListener('click', () => action(async () => {
  const result = await api('library/export', 'POST');
  toast(i18n.t('已匯出至下載資料夾') + '：' + result.path);
}));
$('#import-config').addEventListener('click', () => { if (!busy) $('#import-config-file').click(); });
$('#import-config-file').addEventListener('change', async event => {
  const file = event.target.files[0];
  event.target.value = '';
  if (!file) return;
  await action(async () => {
    if (file.size > 1024 * 1024) throw new Error(i18n.t('設定檔不可超過 1 MB'));
    let library;
    try { library = JSON.parse(await file.text()); } catch { throw new Error(i18n.t('設定檔格式無效')); }
    if (!library || !Array.isArray(library.groups) || !Array.isArray(library.sites)) throw new Error(i18n.t('設定檔格式無效'));
    // 僅匯入站台欄位，連線服務與密碼由本機管理。
    library = { groups: library.groups.map(g => ({ id: g.id, name: g.name })), sites: library.sites.map(s => ({ id: s.id, name: s.name, room: s.room, group: s.group || '', note: s.note || '' })) };
    confirmDelete(i18n.t('匯入站台設定檔'), i18n.t('匯入將取代目前的站台與群組，是否繼續？') + ` (${library.groups.length} / ${library.sites.length})`, async () => {
      await save(library);
      rememberGroup('*'); renderLibrary();
      toast(i18n.t('站台設定已匯入'));
    });
    $('#confirm-form button[type="submit"]').textContent = i18n.t('確認匯入');
  });
});

// 更新檢查在 Go 常駐程序執行，視窗隱藏時仍依 12 小時週期運作。
let releaseShown = '';
function renderRelease(value, manual=false) {
 if (!value) return;
 const dialog=$('#release-dialog');
 const pending=value.available && value.version!==value.notifiedVersion && value.version!==releaseShown;
 if ((pending || (manual && value.available)) && !dialog.open && !document.querySelector('dialog[open]')) {
  releaseShown=value.version;openDialog('#release-dialog');
  api('updates/ack','POST',{version:value.version}).catch(()=>{releaseShown='';});
 }
 $('#release-version').textContent=value.version || '';
 const busy=value.downloading || value.opening;
 const countdown=Number(value.installAt)>0;
 if(countdown && !dialog.open) openDialog('#release-dialog');
 $('#release-later').textContent=i18n.t(countdown?'取消':'稍後');
 $('#release-later').disabled=!!value.opening;
 $('#release-close').disabled=!!value.opening;
 const total=Math.max(0,Number(value.asset?.size)||0);
 const downloaded=Math.min(total,Math.max(0,Number(value.downloadBytes)||0));
 const percent=total?Math.floor(downloaded/total*100):0;
 const downloadLabel=total && downloaded>=total?'正在驗證更新…':'正在下載更新…';
 $('#release-progress-area').hidden=!value.available || !(busy || value.downloadPath || downloaded>0);
 $('#release-progress').value=percent;
 const formatSize=bytes=>bytes>=1024*1024?(bytes/(1024*1024)).toFixed(1)+' MB':(bytes/1024).toFixed(1)+' KB';
 const progressText=`${percent}% · ${formatSize(downloaded)} / ${formatSize(total)}`;
 $('#release-progress-label').textContent=progressText;
 $('#release-progress').setAttribute('aria-valuetext',progressText);
 const openingLabel='正在準備自動安裝…';
 const label=value.downloading?downloadLabel:value.opening?openingLabel:countdown?'立即更新':value.downloadPath?'開始更新':'下載並自動更新';
 $('#release-title').textContent=i18n.t(value.downloadPath?'下載完成':'發現新版本');
 const message=value.downloading?downloadLabel:value.opening?openingLabel:value.downloadError || value.openError || (value.available && value.asset?.browser_download_url?'下載完成後將自動關閉程式、安裝新版並重新啟動。遠端連線會中斷。':value.message);
 $('#release-status').classList.toggle('update-countdown',countdown);
 $('#release-status').textContent=countdown?i18n.t('即將自動更新，可按取消。')+' '+Math.max(0,Math.ceil((Number(value.installAt)-Date.now())/1000))+' s':i18n.t(message || '');
 $('#release-path').textContent=value.downloadPath ? i18n.t('下載目錄：')+'\n'+value.downloadPath : '';
 $('#release-download').disabled=busy || !value.available || !value.asset?.browser_download_url;
 $('#release-download').textContent=i18n.t(label);
 $('#release-download').classList.toggle('danger',!!value.downloadPath);
 $('#release-download').classList.toggle('primary',!value.downloadPath);
 $('#download-update').disabled=busy;
 $('#download-update').textContent=i18n.t(label);
}
async function downloadUpdate() {
 const result=await api(state?.updates?.installAt?'updates/install-now':'updates/download','POST');
 state.updates=result;
 if (!$('#release-dialog').open) openDialog('#release-dialog');
 renderRelease(result);
}
$('#release-download').addEventListener('click',()=>action(async()=>{
 await downloadUpdate();
}));
async function cancelUpdateCountdown(){
 await api('updates/cancel','POST');
 if(state?.updates)state.updates.installAt=0;
 $('#release-dialog').close();
}
$('#release-close').addEventListener('click',()=>action(cancelUpdateCountdown));
$('#release-later').addEventListener('click',()=>action(cancelUpdateCountdown));
$('#release-dialog').addEventListener('cancel',e=>{e.preventDefault();if(!state?.updates?.opening)action(cancelUpdateCountdown);});
window.addEventListener('yourdesk-update',async()=>{
 try { const value=await api('updates/state'); if(state){state.updates=value;renderRelease(value);} } catch {}
});

// 下載期間只輪詢輕量進度 API，不重做 GitHub 更新檢查。
let releaseProgressPolling=false;
setInterval(async()=>{
 if(releaseProgressPolling || !state?.updates || !(state.updates.downloading || state.updates.opening || state.updates.installAt))return;
 releaseProgressPolling=true;
 try {const value=await api('updates/state');state.updates=value;renderRelease(value);}catch{}finally{releaseProgressPolling=false;}
},500);

function refreshSuperResolution(selected) {
 const select=$('#ui-super-resolution');select.replaceChildren();
 const capabilities=state?.superResolutionCapabilities||[{id:'fsr1',name:'FSR 1 · EASU + RCAS',available:true},{id:'coreml',name:'Core ML',available:false,reason:'正在偵測'}];
 for(const available of [true,false]) {
  const group=document.createElement('optgroup');group.label=i18n.t(available?'可使用':'不支援');
  for(const capability of capabilities.filter(c=>c.available===available)) {
   const option=document.createElement('option');option.value=capability.id;option.textContent=capability.name;option.disabled=!available;group.append(option);
  }
  if(!group.children.length) {
   const option=document.createElement('option');option.value='';option.textContent=i18n.t('無');option.disabled=true;group.append(option);
  }
  select.append(group);
 }
 select.value=selected||'fsr1';
 const current=capabilities.find(c=>c.id===select.value);
 select.title=current?.reason?i18n.t(current.reason):'';
 renderCoreMLModelRow();
}

function renderEnhancementStrategy(){
 const mode=$('#ui-enhancement-strategy').value;
 if(mode==='quality'&&!$('#ui-enhancement-budget').checkValidity())$('#ui-enhancement-budget').value=12;
 const budget=Number($('#ui-enhancement-budget').value)||12;
 $('#enhancement-budget-row').hidden=mode==='quality';
 $('#enhancement-rate-summary').textContent=mode==='quality'?i18n.t('沿用原畫質設定，不額外降低'):String(mode==='traffic'?budget/2:budget)+' Mbps';
}

function renderCoreMLModelRow(){
 $('#coreml-model-row').hidden=$('#ui-super-resolution').value!=='coreml';
}
function refreshCoreMLModels(selected){
 const select=$('#ui-coreml-model');select.replaceChildren();
 const models=state?.coreMLModels||[
  {id:'quicksrnet-small',name:'QuickSRNet Small 2×',available:false,reason:'正在偵測'},
  {id:'sesr-m5',name:'SESR M5 2×',available:false,reason:'正在偵測'}
 ];
 for(const model of models){
  const option=document.createElement('option');option.value=model.id;option.textContent=model.name;
  option.disabled=!model.available;option.title=model.reason?i18n.t(model.reason):'';select.append(option);
 }
 select.value=selected||'quicksrnet-small';
 const current=models.find(m=>m.id===select.value);select.title=current?.reason?i18n.t(current.reason):'';
 renderCoreMLModelRow();
}

function renderInterpolationSupport(){
 const supported=state?.info?.platform==='darwin'&&state?.info?.architecture==='arm64';
 const control=$('#ui-interpolation');control.disabled=!supported;
 control.title=supported?'':i18n.t('RIFE 目前僅支援 Apple Silicon Mac');
 const method=$('#ui-interpolation-method');method.disabled=!supported;
 method.querySelector('[value=apple]').disabled=!state?.appleInterpolationSupported;
 method.title=state?.appleInterpolationSupported?'':i18n.t('Apple 補幀需要支援的 Mac 與 macOS 26 以上');
}

let streamAutoRun = null;
function setStreamAutoStep(index, progress, message) {
 $('#stream-auto-hint').hidden = index >= 2;
 $('#stream-auto-progress').value = progress;
 $('#stream-auto-message').textContent = message;
 [...$('#stream-auto-steps').children].forEach((step,i)=>{
  step.dataset.state=i<index?'done':i===index?'active':'waiting';
  if(i===index)step.setAttribute('aria-current','step');else step.removeAttribute('aria-current');
 });
}
function closeStreamAuto() {
 if(streamAutoRun?.applying) return;
 if(streamAutoRun) { streamAutoRun.cancelled=true; streamAutoRun.resolveStart?.(false); streamAutoRun.resolveApply?.(false); }
 $('#stream-auto-dialog').close();
}
$('#stream-auto-close').addEventListener('click',closeStreamAuto);
$('#stream-auto-cancel').addEventListener('click',closeStreamAuto);
$('#stream-auto-dialog').addEventListener('cancel',event=>{event.preventDefault();closeStreamAuto();});
$('#stream-auto-dialog').addEventListener('close',()=>{if(streamAutoRun&&!streamAutoRun.applying){streamAutoRun.cancelled=true;streamAutoRun.resolveStart?.(false); streamAutoRun.resolveApply?.(false);}});
$('#stream-auto-start').addEventListener('click',()=>{
 const run=streamAutoRun;
 if(!run?.resolveStart||run.cancelled)return;
 $('#stream-auto-start').hidden=true;
 run.resolveStart(true);run.resolveStart=null;
});
$('#stream-auto-apply').addEventListener('click',()=>{
 const run=streamAutoRun;
 if(!run?.resolveApply||run.cancelled)return;
 $('#stream-auto-apply').hidden=true;
 run.resolveApply(true);run.resolveApply=null;
});
async function autoConfigureStream(mode) {
 if (busy || streamAutoRun) return;
 const run={cancelled:false,applying:false};streamAutoRun=run;
 const buttons=[];
 buttons.forEach(button=>button.disabled=true);
 $('#settings-dialog').close();
 $('#stream-auto-message').dataset.error='false';
 $('#stream-auto-result').replaceChildren();
 $('#stream-auto-cancel').textContent=i18n.t('取消');
 $('#stream-auto-start').hidden=false;
 $('#stream-auto-apply').hidden=true;
 setStreamAutoStep(-1,0,i18n.t('準備好後，請按「開始」。'));
 const ready=new Promise(resolve=>{run.resolveStart=resolve;});
 openDialog('#stream-auto-dialog');
 $('#stream-auto-start').focus({preventScroll:true});
 const started=await ready;
 run.resolveStart=null;
 if(!started||run.cancelled){
  streamAutoRun=null;
  buttons.forEach(button=>button.disabled=false);
  return;
 }
 setStreamAutoStep(0,0,i18n.t('確認遠端連線'));
 await action(async()=>{
  try {
   const id=typeof diagnosticSite!=='undefined' && diagnosticSite && Object.hasOwn(state.running,`viewer:${diagnosticSite.id}`) ? diagnosticSite.id : '';
   let result=await api('stream-auto','POST',{mode,id});
   run.probe={mode,id:result.id,startedAt:result.startedAt};
   const deadline=performance.now()+24000;
   while(result.pending) {
    if(run.cancelled)return;
    const seconds=Number.isFinite(result.sampleSeconds)?result.sampleSeconds:0;
    setStreamAutoStep(1,10+Math.min(65,seconds/10*65),`${i18n.t('收集有效畫面樣本')} · ${seconds.toFixed(1)} / 10 s`);
    await new Promise(resolve=>setTimeout(resolve,1000));
    if(run.cancelled)return;
    if(performance.now()>deadline)throw new Error(i18n.t('有效樣本不足，設定未變更；請確認遠端畫面有變化，或改用有畫面的遠端連線後重試。'));
    result=await api('stream-auto','POST',{mode,id:result.id,startedAt:result.startedAt});
   }
   if(run.cancelled)return;
   setStreamAutoStep(2,80,i18n.t('計算建議配置'));
   const details=$('#stream-auto-result');
   const add=(label,value)=>{const line=text('div','','diagnostic-row');line.append(text('span',i18n.t(label)),text('strong',value));details.append(line);};
   add('動態樣本平均',`${result.measuredFPS.toFixed(1)} FPS`);
   add('有效動態取樣',`${result.activeSeconds.toFixed(1)} s`);
   add('建議來源 FPS',`${result.preferences.sourceFPSLimit} FPS`);
   add('GOP（關鍵影格間隔）',String(result.preferences.keyframeInterval));
   await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
   if(run.cancelled)return;
   setStreamAutoStep(3,85,i18n.t('偵測完成，按「套用」才會變更設定。'));
   const lastStep=$('#stream-auto-steps').lastElementChild;
   lastStep.dataset.state='waiting';lastStep.removeAttribute('aria-current');
   $('#stream-auto-apply').hidden=false;
   const confirmed=await new Promise(resolve=>{run.resolveApply=resolve;});
   run.resolveApply=null;
   if(!confirmed||run.cancelled)return;
   run.applying=true;
   $('#stream-auto-close').disabled=true;$('#stream-auto-cancel').disabled=true;
   setStreamAutoStep(3,90,i18n.t('套用並儲存設定'));
   result=await api('stream-auto','POST',{mode,id:result.id,startedAt:result.startedAt,apply:true});
   state.preferences=result.preferences;applyPreferences(result.preferences);
   setStreamAutoStep(4,100,i18n.t('設定已儲存'));
   details.replaceChildren();
   add('動態樣本平均',`${result.measuredFPS.toFixed(1)} FPS`);
   add('有效動態取樣',`${result.activeSeconds.toFixed(1)} s`);
   add('來源 FPS 上限',`${result.preferences.sourceFPSLimit} FPS`);
   add('GOP（關鍵影格間隔）',String(result.preferences.keyframeInterval));
   add('碼率上限（Mbps）',`${result.preferences.bitrateLimitMbps} Mbps`);
  } catch(error) {
   if(!run.cancelled){$('#stream-auto-message').dataset.error='true';$('#stream-auto-message').textContent=i18n.t(error.message);}
  } finally {
   $('#stream-auto-apply').hidden=true;
   if(run.probe) { try { await api('stream-auto','POST',{...run.probe,cancel:true}); } catch {} }
   run.applying=false;
   $('#stream-auto-close').disabled=false;$('#stream-auto-cancel').disabled=false;
   $('#stream-auto-cancel').textContent=i18n.t('關閉');
  }
 });
 streamAutoRun=null;
 buttons.forEach(button=>button.disabled=false);
}

let lastHostConflict = '';
function showHostConflict(owner) {
 if(!owner){lastHostConflict='';return;}
 const key=JSON.stringify(owner);
 if(key===lastHostConflict||document.querySelector('dialog[open]'))return;
 lastHostConflict=key;
 confirmDelete(i18n.t('發現重複的 Host 程序'),
   i18n.t('是否停止下列程序？該程序目前的遠端連線將會中斷。')+'\nPID: '+owner.pid+'\n'+owner.path,
   async()=>{await api('host-conflict/stop','POST',owner);});
 $('#confirm-form button[type="submit"]').textContent=i18n.t('停止程序');
}

$('#copy-mcp-address').addEventListener('click',()=>action(async()=>{await navigator.clipboard.writeText($('#mcp-address').textContent);toast(i18n.t('MCP 位址已複製'));}));

let preloginWasBusy=false;
// 服務狀態由系統安裝結果決定，不存成一般偏好值。
function renderPrelogin() {
 const service=state?.prelogin;
 const toggle=$('#ui-prelogin');
 toggle.checked=!!service?.enabled;
 toggle.disabled=!service?.supported||!!service?.busy;
 const row=toggle.closest('.preference-row');
 row.classList.toggle('experimental-unavailable',toggle.disabled);
 row.dataset.tooltip=i18n.t(service?.message||'這台電腦目前不支援登入前連線。');
 $('#prelogin-access-reason').textContent=row.dataset.tooltip;
 $('#prelogin-progress').textContent=service?.busy ? i18n.t(service.message) : '';
 if (!service?.busy && preloginWasBusy && service?.error) toast(i18n.t(service.error),false,{warning:true,duration:15000});
 preloginWasBusy=!!service?.busy;
}
$('#ui-prelogin').addEventListener('change',()=>action(async()=>{
 const toggle=$('#ui-prelogin');
 const enabled=toggle.checked;
 toggle.disabled=true;
 try {await api('prelogin','POST',{enabled});await updateRunning();}
 finally {renderPrelogin();}
}));

$('#stop-incoming').addEventListener('click',()=>action(async()=>{
 const button=$('#stop-incoming');button.disabled=true;
 try {await api('incoming/disconnect','POST',{});await updateRunning();}
 finally {button.disabled=false;}
}));

$('#tailcat-mode').addEventListener('change',savePreferences);

window.addEventListener("yourdesk-siri-site", event => {
 if(typeof event.detail!=="string"||!state)return;
 selectedGroup="*"; $("#search").value=event.detail; renderLibrary();
});
