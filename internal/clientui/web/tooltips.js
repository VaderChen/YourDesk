'use strict';
// 共用提示元件：同時支援滑鼠與鍵盤，並放在目前對話框內避免被遮住。
const hints = (() => {
  const bubble = document.createElement('div');
  bubble.id = 'app-tooltip';
  bubble.className = 'app-tooltip';
  bubble.setAttribute('role', 'tooltip');
  bubble.hidden = true;
  let enabled = true;
  let target = null;
  let timer;
  function hide() {
    clearTimeout(timer);
    if (target) {
      const ids = (target.getAttribute('aria-describedby') || '').split(/\s+/).filter(id => id && id !== bubble.id);
      if (ids.length) target.setAttribute('aria-describedby', ids.join(' '));
      else target.removeAttribute('aria-describedby');
    }
    target = null;
    bubble.hidden = true;
  }
  function show(element) {
    if (element === target) return;
    hide();
    if (!enabled || !element?.dataset.tooltip) return;
    target = element;
    timer = setTimeout(() => {
      if (!target?.isConnected) { hide(); return; }
      bubble.textContent = target.dataset.tooltip;
      (target.closest('dialog[open]') || document.body).append(bubble);
      bubble.hidden = false;
      const rect = target.getBoundingClientRect();
      const width = bubble.offsetWidth, height = bubble.offsetHeight;
      const below = rect.top < height + 16;
      bubble.dataset.side = below ? 'below' : 'above';
      const left = Math.max(8, Math.min(rect.left + rect.width / 2 - width / 2, innerWidth - width - 8));
      bubble.style.left = `${left}px`;
      bubble.style.top = `${Math.max(8, Math.min(below ? rect.bottom + 9 : rect.top - height - 9, innerHeight - height - 8))}px`;
      bubble.style.setProperty('--arrow-x', `${Math.max(12, Math.min(rect.left + rect.width / 2 - left, width - 12))}px`);
      const ids = (target.getAttribute('aria-describedby') || '').split(/\s+/).filter(Boolean);
      target.setAttribute('aria-describedby', [...new Set([...ids, bubble.id])].join(' '));
    }, 0);
  }
  document.addEventListener('pointerover', event => show(event.target.closest('[data-tooltip]')));
  document.addEventListener('pointerout', event => { if (target && !target.contains(event.relatedTarget)) hide(); });
  document.addEventListener('focusin', event => show(event.target.closest('[data-tooltip]')));
  document.addEventListener('focusout', hide);
  document.addEventListener('pointerdown', hide, true);
  document.addEventListener('dragstart', hide, true);
  document.addEventListener('scroll', hide, true);
  document.addEventListener('close', hide, true);
  document.addEventListener('keydown', event => { if (event.key === 'Escape') hide(); });
  window.addEventListener('resize', hide);
  window.addEventListener('blur', hide);
  new MutationObserver(() => { if (target && (!target.isConnected || !target.getClientRects().length)) hide(); }).observe(document.body, { childList: true, subtree: true });
  return { setEnabled(value) { enabled = value; hide(); } };
})();
