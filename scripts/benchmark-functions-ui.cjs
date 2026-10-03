'use strict';
// Real application functions in a minimal DOM harness: measures JS work only,
// excluding browser layout, paint, networking, and user input. No dependencies.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {performance} = require('node:perf_hooks');
const sourceArg = process.argv.find(arg => arg.startsWith('--source='));
const source = sourceArg ? path.resolve(sourceArg.slice(9)) : path.resolve(__dirname, '../internal/clientui/web');
const app = fs.readFileSync(path.join(source, 'app.js'), 'utf8');
const files = fs.readFileSync(path.join(source, 'files.js'), 'utf8');

function functionSource(text, name, optional = false) {
  const start = new RegExp(`^([ ]*)function ${name}\\(`, 'm').exec(text);
  if (!start && optional) return '';
  assert.ok(start, `Missing application function ${name}`);
  const end = text.indexOf('\n' + start[1] + '}', start.index);
  assert.ok(end > start.index, `Missing function boundary for ${name}`);
  return text.slice(start.index, end + start[1].length + 2);
}

class Element {
  constructor(tag) { this.tag = tag; this.children = []; this.dataset = {}; this.classList = {toggle() {}}; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  setAttribute() {}
  addEventListener() {}
}
const nodes = new Map();
const node = id => { if (!nodes.has(id)) nodes.set(id, new Element(id)); return nodes.get(id); };
let cards = [];
const context = vm.createContext({
  state: {queue: [], entries: [], selected: new Set(), library: {sites: []}},
  document: {createElement: tag => new Element(tag), querySelectorAll: () => cards},
  $: node, locale: 'en', parentPath: null, controls() {},
  t: key => key, sizeText: size => String(size), progressText: (...args) => JSON.stringify(args),
  applySiteDeviceStatus(icon, site) { icon.status = site?.name; },
});
vm.runInContext([
  functionSource(app, 'indexById', true), functionSource(app, 'refreshSiteDeviceStatuses'),
  functionSource(files, 'queueSummary'), functionSource(files, 'renderList'),
].join('\n'), context);

function sitesFixture(size) {
  context.state.library.sites = Array.from({length: size}, (_, i) => ({id: String(i), name: `site-${i}`}));
  cards = context.state.library.sites.map(site => ({dataset: {siteId: site.id}, icon: {}, querySelector() { return this.icon; }}));
}
sitesFixture(20);
context.state.library.sites.push({id: '0', name: 'duplicate must not replace first'});
context.refreshSiteDeviceStatuses();
assert.deepEqual(cards.map(card => card.icon.status), Array.from({length: 20}, (_, i) => `site-${i}`));
context.state.library.sites[0] = {id: '0', name: 'changed in same array'};
context.refreshSiteDeviceStatuses();
assert.equal(cards[0].icon.status, 'changed in same array');
context.state.library.sites = [];
context.refreshSiteDeviceStatuses();
assert.ok(cards.every(card => card.icon.status === undefined));

for (const queue of [[], [{directory: true}], [{state: 'cancelled', size: 90}],
  [{state: 'done', size: 0}], [{state: 'done', size: 3, transfer: {offset: 3}}],
  [{state: 'running', size: 20, transfer: {offset: 3}}, {state: 'queued', size: 10},
    {state: 'cancelled', size: 99}, {directory: true, size: 99}]]) {
  context.state.queue = queue;
  context.queueSummary();
  const included = queue.filter(item => !item.directory && item.state !== 'cancelled');
  const total = included.reduce((sum, item) => sum + item.size, 0);
  const received = included.reduce((sum, item) => sum + (item.transfer?.offset || 0), 0);
  const complete = included.length > 0 && included.every(item => item.state === 'done');
  assert.equal(node('queue-summary').hidden, !included.length);
  assert.equal(node('queue-progress').hidden, !included.length);
  assert.equal(node('queue-summary').textContent, JSON.stringify([received, total, complete]));
  assert.equal(node('queue-progress').max, total || 1);
  assert.equal(node('queue-progress').value, received || (complete ? 1 : 0));
}

const dates = ['2026-10-03T01:23:45Z', '2000-02-29T23:59:59Z', '2026-03-08T09:59:59Z', 'invalid', '', null];
for (const locale of ['zh-Hant', 'en', 'ja', 'ko']) {
  context.locale = locale;
  context.state.entries = dates.map((modified, i) => ({path: String(i), name: `file-${i}`, size: 1, modified}));
  context.renderList();
  const displayed = node('files-list').children.slice(1).map(row => row.children[2].textContent);
  assert.deepEqual(displayed, dates.map(value => {
    const date = new Date(value);
    return value && !Number.isNaN(date.getTime()) ? date.toLocaleDateString(locale) : '—';
  }));
}
console.log('UI function regression PASS: fresh/duplicate site IDs, queue states, four date locales');

if (process.argv.includes('--bench')) {
  function bench(name, run, iterations) {
    for (let i = 0; i < 5; i++) run();
    for (let round = 1; round <= 3; round++) {
      const started = performance.now();
      for (let i = 0; i < iterations; i++) run();
      console.log(JSON.stringify({name, round, msPerCall: (performance.now() - started) / iterations}));
    }
  }
  sitesFixture(5000);
  bench('refreshSiteDeviceStatuses/5000', () => context.refreshSiteDeviceStatuses(), 20);
  context.locale = 'zh-Hant';
  context.state.entries = Array.from({length: 1000}, (_, i) => ({path: String(i), name: `file-${i}`, size: 100, modified: dates[i % 3]}));
  bench('renderList/1000', () => context.renderList(), 10);
  context.state.queue = Array.from({length: 2000}, () => ({state: 'running', size: 100, transfer: {offset: 50}}));
  bench('queueSummary/2000', () => context.queueSummary(), 1000);
}
