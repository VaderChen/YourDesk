'use strict';
// 固定介面字串集中管理；使用者輸入的站台名稱、備註及識別碼保持原文。
const i18n = (() => {
  let dictionary = {};
  let locale = 'zh-Hant';
  let preference = 'auto';
  const columns = { en: 0, ja: 1, ko: 2 };
  let templates = [];
  const staticText = [];
  const staticAttributes = [];
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) {
    const node = walker.currentNode;
    if (node.parentElement?.closest('script,style,[data-no-i18n]')) continue;
    if (node.textContent.trim()) staticText.push({ node, source: node.textContent });
  }
  for (const element of document.querySelectorAll('[placeholder],[aria-label],[data-tooltip]')) {
    for (const name of ['placeholder', 'aria-label', 'data-tooltip']) {
      if (element.hasAttribute(name)) staticAttributes.push({ element, name, source: element.getAttribute(name) });
    }
  }
  function systemLanguage() {
    for (const language of navigator.languages || [navigator.language]) {
      if (/^zh/i.test(language)) return 'zh-Hant';
      if (/^ja/i.test(language)) return 'ja';
      if (/^ko/i.test(language)) return 'ko';
      if (/^en/i.test(language)) return 'en';
    }
    return 'en';
  }
  function t(value) {
    if (typeof value !== 'string' || locale === 'zh-Hant') return value;
    const source = value.trim();
    let translated = dictionary[source]?.[columns[locale]];
    if (translated === undefined) {
      for (const entry of templates) {
        const match = source.match(entry.pattern);
        if (match) { translated = entry.values[columns[locale]].replace('{0}', match[1]); break; }
      }
    }
    return translated === undefined ? value : value.replace(source, translated);
  }
  async function load() {
    const response = await fetch('translations.json');
    if (!response.ok) throw new Error('無法載入介面語言');
    dictionary = await response.json();
    templates = Object.entries(dictionary).filter(([key]) => key.includes('{0}')).map(([key,values]) => ({
      pattern: new RegExp('^' + key.split('{0}').map(part => part.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')).join('(.+?)') + '$'), values
    }));
  }
  function apply(language) {
    preference = language;
    locale = language === 'auto' ? systemLanguage() : language;
    document.documentElement.lang = locale;
    for (const {node,source} of staticText) if (node.isConnected) node.textContent = t(source);
    for (const {element,name,source} of staticAttributes) if (element.isConnected) element.setAttribute(name,t(source));
  }
  return { load, apply, t, get preference() { return preference; } };
})();
