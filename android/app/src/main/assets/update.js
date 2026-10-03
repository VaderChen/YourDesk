'use strict';
// 沿用首頁版面；只有首頁前景且沒有操作中的對話框時，才可下載／提示更新。
(() => {
  const report = () => window.YourDesk?.setUpdateState?.(
    !document.hidden && !document.querySelector('.overlay:not([hidden])'),
    document.documentElement.lang || 'zh-Hant');
  new MutationObserver(report).observe(document.body, {
    attributes: true, subtree: true, attributeFilter: ['hidden']
  });
  new MutationObserver(report).observe(document.documentElement, {
    attributes: true, attributeFilter: ['lang']
  });
  document.addEventListener('visibilitychange', report);
  window.addEventListener('pageshow', report);
  window.addEventListener('pagehide', () => window.YourDesk?.setUpdateState?.(false, document.documentElement.lang));
  report();
})();
