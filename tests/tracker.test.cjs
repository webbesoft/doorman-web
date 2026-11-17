const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const source = fs.readFileSync(new URL('../assets/js/t.js', `file://${__filename}`), 'utf8');

function browser(options = {}) {
  let now = 0, id = 0, random = 0;
  const timers = new Map(), docEvents = {}, winEvents = {}, sends = [];
  const document = { readyState: options.readyState || 'complete', hidden: false, referrer: 'https://search.example/?private=secret',
    currentScript: { src: 'https://analytics.example/assets/js/t.js', dataset: {} },
    documentElement: { scrollHeight: options.height || 1000, scrollTop: 0 },
    addEventListener(name, fn) { (docEvents[name] ||= []).push(fn); } };
  const window = { location: { href: 'https://site.example/one?token=secret#private' }, innerHeight: 500, scrollY: 0,
    crypto: { getRandomValues(bytes) { bytes.fill(++random); } },
    requestAnimationFrame(fn) { timers.set(++id, { fn, at: now + 16 }); return id; },
    addEventListener(name, fn) { (winEvents[name] ||= []).push(fn); },
    fetch(endpoint, request) { sends.push(JSON.parse(request.body)); return Promise.resolve({ status: options.status || 204 }); } };
  const history = {};
  for (const name of ['pushState', 'replaceState']) history[name] = function (_, __, path) { window.location.href = new URL(path, window.location.href).href; };
  const context = vm.createContext({ window, document, history, URL, Uint8Array, performance: { now: () => now },
    navigator: { doNotTrack: options.doNotTrack, globalPrivacyControl: options.gpc,
      sendBeacon(endpoint, body) { sends.push(JSON.parse(body)); return true; } },
    setTimeout(fn, delay) { timers.set(++id, { fn, at: now + delay }); return id; },
    setInterval(fn, delay) { timers.set(++id, { fn, at: now + delay, repeat: delay }); return id; } });
  const emit = (events, name, payload = {}) => (events[name] || []).forEach(fn => fn(payload));
  function advance(ms) {
    const end = now + ms;
    for (;;) {
      const next = [...timers].filter(([, t]) => t.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
      if (!next) break;
      const [key, timer] = next; now = timer.at;
      if (timer.repeat) timer.at += timer.repeat; else timers.delete(key);
      timer.fn();
    }
    now = end;
  }
  vm.runInContext(source, context);
  return { sends, document, window, history, advance, reload: () => vm.runInContext(source, context),
    doc: (name, payload) => emit(docEvents, name, payload), win: (name, payload) => emit(winEvents, name, payload) };
}

test('records immediately, strips URL secrets, and prevents duplicate installation', () => {
  const b = browser(); assert.equal(b.sends.length, 1); assert.equal(b.sends[0].url, 'https://site.example/one');
  assert.equal(b.sends[0].referrer, 'https://search.example'); b.reload(); assert.equal(b.sends.length, 1);
});
test('hidden time is excluded from active time', () => {
  const b = browser(); b.advance(6000); b.document.hidden = true; b.doc('visibilitychange'); b.advance(10000); b.win('pagehide');
  assert.equal(b.sends.at(-1).dwellTime, 16); assert.equal(b.sends.at(-1).activeTime, 6);
});
test('a cached page starts a new navigation without counting time spent away', () => {
  const b = browser(); const id = b.sends[0].visitId; b.advance(6000); b.win('pagehide', { persisted: true });
  b.advance(60000); b.win('pageshow', { persisted: true }); const restored = b.sends.length; b.advance(30000);
  assert.ok(b.sends.length > restored); assert.notEqual(b.sends.at(-1).visitId, id); assert.equal(b.sends[restored - 1].dwellTime, 0);
});
test('rapid SPA transitions each record a distinct navigation', () => {
  const b = browser(); b.history.pushState({}, '', '/two'); b.history.pushState({}, '', '/three');
  b.advance(32);
  const initial = [...new Map(b.sends.map(x => [x.visitId, x])).values()];
  assert.deepEqual(initial.map(x => x.url), ['https://site.example/one', 'https://site.example/two', 'https://site.example/three']);
  assert.equal(new Set(initial.map(x => x.visitId)).size, 3);
});
test('only one final event is sent and query-only navigation creates no visit', () => {
  const b = browser(); b.history.replaceState({}, '', '/one?other=secret'); assert.equal(b.sends.length, 1);
  b.win('beforeunload'); b.win('pagehide'); b.win('pagehide'); assert.equal(b.sends.filter(x => x.final).length, 1);
});
test('idle active time is capped and privacy signals disable collection', () => {
  const b = browser(); b.advance(90000); assert.equal(b.sends.at(-1).activeTime, 30);
  assert.equal(browser({ doNotTrack: '1' }).sends.length, 0); assert.equal(browser({ gpc: true }).sends.length, 0);
});
test('failed deliveries retry the same event without creating a new visit', async () => {
  const b = browser({ status: 503 }); await Promise.resolve(); b.advance(1000); await Promise.resolve(); b.advance(2000); await Promise.resolve();
  assert.equal(b.sends.length, 3); assert.equal(new Set(b.sends.map(x => x.visitId)).size, 1); assert.equal(new Set(b.sends.map(x => x.sequence)).size, 1);
});


test('initial coverage waits for full layout while the page view sends immediately', () => {
  const b = browser({ readyState: 'loading', height: 100 });
  assert.equal(b.sends.length, 1);
  assert.equal(b.sends[0].scrollDepth, 0);
  b.document.documentElement.scrollHeight = 2000;
  b.document.readyState = 'complete'; b.win('load'); b.advance(32); b.win('pagehide');
  assert.equal(b.sends.at(-1).scrollDepth, 25);
});
test('deferred initialization preserves later scrolling', () => {
  const b = browser({ readyState: 'loading', height: 100 });
  b.document.documentElement.scrollHeight = 2000; b.window.scrollY = 1000; b.win('scroll');
  b.document.documentElement.scrollHeight = 4000;
  b.document.readyState = 'complete'; b.win('load'); b.advance(32); b.win('pagehide');
  assert.equal(b.sends.at(-1).scrollDepth, 75);
});
test('SPA initial events include the new layout coverage and skip stale callbacks', () => {
  for (const [height, depth] of [[400, 100], [2000, 25]]) {
    const b = browser(); b.advance(32);
    b.history.pushState({}, '', '/two'); b.document.documentElement.scrollHeight = height;
    b.advance(32);
    const initial = b.sends.find(x => x.url.endsWith('/two') && !x.final);
    assert.ok(initial); assert.equal(initial.scrollDepth, depth);
  }
  const b = browser(); b.history.pushState({}, '', '/two'); b.history.pushState({}, '', '/three');
  b.document.documentElement.scrollHeight = 2000; b.advance(32);
  assert.equal(b.sends.filter(x => x.url.endsWith('/two') && !x.final).length, 0);
  assert.equal(b.sends.filter(x => x.url.endsWith('/three') && !x.final).length, 1);
});
