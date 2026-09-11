'use strict';
let diagnosticSite = null;
let diagnosticEpoch = 0;
let diagnosticTimer;
let diagnosticActiveRun = null;
function stopDiagnosticRun() {
 const run=diagnosticActiveRun;diagnosticActiveRun=null;
 if(run) api(`diagnostics?id=${encodeURIComponent(run.id)}&startedAt=${run.startedAt}`,'DELETE').catch(()=>{});
}
const diagnosticDialog = $('#diagnostic-dialog');
$('#connect-dialog').addEventListener('close', () => { if (!connectionWait) pendingDiagnosticSite = null; });
diagnosticDialog.addEventListener('close', () => { diagnosticEpoch++; clearTimeout(diagnosticTimer); stopDiagnosticRun(); });
$('#diagnostic-retry').addEventListener('click', () => { if (diagnosticSite) openDiagnostics(diagnosticSite); });

async function openDiagnostics(site) {
  const epoch = ++diagnosticEpoch;
  clearTimeout(diagnosticTimer);
  diagnosticSite = site;
  $('#diagnostic-site').textContent = `${site.name} · ${site.room}`;
  $('#diagnostic-status').textContent = i18n.t('正在準備分析…');
  $('#diagnostic-report').replaceChildren();
  $('#diagnostic-progress').hidden = false;
  $('#diagnostic-progress').value = 0;
  $('#diagnostic-retry').disabled = true;
  if (!diagnosticDialog.open) openDialog('#diagnostic-dialog');
  const path = `diagnostics?id=${encodeURIComponent(site.id)}`;
  try {
    const initial = await api(path, 'POST');
    if (epoch !== diagnosticEpoch) return;
    if (!initial.connected) {
      diagnosticDialog.close();
      pendingDiagnosticSite = site;
      connect(site);
      return;
    }
    const started = initial.run.startedAt;
    diagnosticActiveRun={id:site.id,startedAt:started};
    const deadline = performance.now() + 22000;
    $('#diagnostic-status').textContent = i18n.t('正在背景分析，約需 10～15 秒…');
    async function poll() {
      if (epoch !== diagnosticEpoch || !diagnosticDialog.open) return;
      try {
        const result = await api(path);
        if (epoch !== diagnosticEpoch) return;
        if (!result.connected || result.run?.startedAt !== started) throw new Error('連線已改變，請重新分析。');
        const samples = result.run.samples || [];
        const seconds = samples.reduce((sum, sample) => sum + sample.seconds, 0);
        $('#diagnostic-progress').value = Math.min(10, seconds);
        if (seconds >= 9.5) {
          showDiagnosticReport(samples);
          finish();
        } else if (performance.now() >= deadline) {
          throw new Error('本次取樣資料不足，請稍後重新分析。');
        } else diagnosticTimer = setTimeout(poll, 1000);
      } catch (error) { if (epoch === diagnosticEpoch) failed(error); }
    }
    diagnosticTimer = setTimeout(poll, 1000);
  } catch (error) { if (epoch === diagnosticEpoch) failed(error); }
  function finish() { stopDiagnosticRun(); $('#diagnostic-progress').hidden = true; $('#diagnostic-retry').disabled = false; }
  function failed(error) { $('#diagnostic-status').textContent = i18n.t(error.message); finish(); }
}

function showDiagnosticReport(samples) {
  const seconds = samples.reduce((sum, sample) => sum + sample.seconds, 0);
  const average = key => samples.reduce((sum, sample) => sum + sample[key] * sample.seconds, 0) / seconds;
  const sum = key => samples.reduce((total, sample) => total + sample[key], 0);
  const rtts = samples.map(sample => sample.rttMs).filter(value => Number.isFinite(value));
  const rtt = rtts.length ? rtts.reduce((a, b) => a + b, 0) / rtts.length : null;
  const background = samples.some(sample => sample.background);
  const fps = average(background?'decodedPerSec':'sourceFps'), received = average('receivedPerSec'), limit = samples.at(-1).fpsLimit;
  const gaps = sum('gaps'), errors = sum('errors');
  const notes = [];
  let network = '回應順暢';
  if (rtt === null) network = '暫無延遲資料';
  else if (rtt >= 150) { network = '回應較慢'; notes.push('可試用有線網路，並暫停兩端的大量下載或上傳。'); }
  else if (rtt >= 60) network = '回應稍有延遲';
  if (rtts.length > 1 && Math.max(...rtts) - Math.min(...rtts) >= 40) {
    network = '回應不太穩定'; notes.push('網路回應有起伏，可先靠近 Wi-Fi 基地台或改用有線網路。');
  }
  if (gaps) notes.push('收到的畫面有缺漏，可先降低碼率上限，再重新分析。');
  if (errors) notes.push('部分畫面無法正常讀取，可重新連線，並確認兩端皆為新版。');
  const jpeg = samples.some(sample => sample.codec.startsWith('JPEG'));
  let stream = '取樣期間正常';
  if (!sum('received')) { stream = '未收到新畫面'; notes.push('本次未收到新畫面，可能是遠端桌面靜止，暫時無法判斷畫面更新速度。'); }
  else if (gaps || errors) stream = '偶有畫面中斷';
  else if (!(background && jpeg) && limit > 0 && fps < limit * 0.7) {
    stream = '畫面更新較慢';
    if (!jpeg && received > fps * 1.4 && received > 5) notes.push(background?'解碼速度較慢，可降低來源 FPS 後比較。':'收到畫面的速度高於顯示速度，可先關閉畫面增強與補幀比較。');
    else notes.push('取樣期間畫面更新較少，可能與桌面內容、遠端處理速度或網路有關。');
  }
  if (jpeg) notes.push('目前使用圖片串流，桌面靜止時更新較少是正常的。');
  if (!notes.length) notes.push(background?'本次背景收件與解碼正常，可先維持設定。':'目前可先維持設定；若覺得不跟手，可關閉補幀比較。');
  if(background) notes.unshift('背景分析不包含畫面顯示、增強與補幀；桌面靜止時更新較少是正常的。');
  const report = $('#diagnostic-report'); report.replaceChildren();
  const row = (label, value) => {
    const element = text('div', '', 'diagnostic-row');
    element.append(text('span', i18n.t(label)), text('strong', value)); report.append(element);
  };
  const measurement = (value, unit) => Number.isFinite(value) ? `${value.toFixed(1)} ${unit}` : '-';
  row('遠端版本', samples.map(sample=>sample.remoteVersion).filter(Boolean).at(-1)||i18n.t('未提供'));
  row('網路回應', `${i18n.t(network)}${rtt === null ? '' : ` · ${Math.round(rtt)} ms`}`);
  row('目前下載流量', measurement(average('receiveMbps'), 'Mbps'));
  row('畫面表現', i18n.t(stream));
  if(background) row(jpeg?'解碼區塊':'解碼畫面', measurement(fps, jpeg?i18n.t('個／秒'):'FPS'));
  else {
    const renderFps = average('renderFps');
    const value = Number.isFinite(fps) || Number.isFinite(renderFps)
      ? `${Number.isFinite(fps) ? fps.toFixed(1) : '-'} / ${Number.isFinite(renderFps) ? renderFps.toFixed(1) : '-'} FPS` : '-';
    row('真實畫面／顯示', value);
  }
  if (limit > 0) row('設定上限', `${limit} FPS`);
  const list = document.createElement('ul');
  [...new Set(notes)].slice(0, 3).forEach(note => list.append(text('li', i18n.t(note))));
  report.append(list);
  $('#diagnostic-status').textContent = i18n.t('分析完成，以下為本次取樣結果。');
}
