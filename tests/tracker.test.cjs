const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const source = fs.readFileSync(new URL('../assets/js/t.js', `file://${__filename}`), 'utf8');

function browser(options = {}) {
  let now = 0, id = 0, random = 0;
  const timers = new Map(), docEvents = {}, winEvents = {}, sends = [];
  const document = { hidden: false, referrer: 'https://search.example/?private=secret',
    currentScript: { src: 'https://analytics.example/assets/js/t.js', dataset: {} },
    documentElement: { scrollHeight: 1000, scrollTop: 0 },
    addEventListener(name, fn) { (docEvents[name] ||= []).push(fn); } };
  const window = { location: { href: 'https://site.example/one?token=secret#private' }, innerHeight: 500, scrollY: 0,
    crypto: { getRandomValues(bytes) { bytes.fill(++random); } },
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
  return { sends, document, history, advance, reload: () => vm.runInContext(source, context),
    doc: (name, payload) => emit(docEvents, name, payload), win: (name, payload) => emit(winEvents, name, payload) };
}

test('records immediately, strips URL secrets, and prevents duplicate installation', () => {
  const b = browser(); assert.equal(b.sends.length, 1); assert.equal(b.sends[0].url, 'https://site.example/one');
  assert.equal(b.sends[0].referrer, 'https://search.example/'); b.reload(); assert.equal(b.sends.length, 1);
});
test('hidden time is excluded from active time', () => {
  const b = browser(); b.advance(6000); b.document.hidden = true; b.doc('visibilitychange'); b.advance(10000); b.win('pagehide');
  assert.equal(b.sends.at(-1).dwellTime, 16); assert.equal(b.sends.at(-1).activeTime, 6);
});
test('a cached page resumes the same visit and heartbeats after restoration', () => {
  const b = browser(); const id = b.sends[0].visitId; b.advance(6000); b.win('pagehide', { persisted: true });
  b.advance(60000); b.win('pageshow', { persisted: true }); const restored = b.sends.length; b.advance(30000);
  assert.ok(b.sends.length > restored); assert.equal(b.sends.at(-1).visitId, id);
});
test('rapid SPA transitions each record a distinct navigation', () => {
  const b = browser(); b.history.pushState({}, '', '/two'); b.history.pushState({}, '', '/three');
  const initial = b.sends.filter(x => !x.final);
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
