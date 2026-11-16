(function () {
  "use strict";
  if (window.__doormanTracking || navigator.doNotTrack === "1" || navigator.globalPrivacyControl) return;
  if (!window.crypto || !window.crypto.getRandomValues) return;
  window.__doormanTracking = true;

  var script = document.currentScript;
  var endpoint = script && script.dataset.endpoint;
  var trackURL = endpoint || (script && script.src ? new URL(script.src).origin + "/event" : "/event");
  var idleAfter = 30000;
  var maxDuration = 86400000;

  function canonicalURL(raw) {
    var url = new URL(raw, window.location.href);
    return url.origin + url.pathname;
  }
  function newID() {
    var bytes = new Uint8Array(16);
    window.crypto.getRandomValues(bytes);
    return Array.from(bytes, function (value) { return value.toString(16).padStart(2, "0"); }).join("");
  }
  function newVisit(url, referrer) {
    var now = performance.now();
    return { id: newID(), url: url, referrer: referrer, start: now, tick: now,
      lastActivity: now, activeMs: 0, scroll: 0, sequence: 0, hidden: document.hidden, finished: false };
  }
  var visit = newVisit(canonicalURL(window.location.href), document.referrer ? canonicalURL(document.referrer) : "");

  function updateActive() {
    var now = performance.now();
    if (!visit.hidden && !visit.finished) {
      visit.activeMs += Math.max(0, Math.min(now, visit.lastActivity + idleAfter) - visit.tick);
    }
    visit.tick = now;
  }
  function markActive() {
    updateActive();
    visit.lastActivity = performance.now();
  }
  function updateScroll() {
    var height = document.documentElement.scrollHeight;
    if (height > 0) {
      var top = window.scrollY || document.documentElement.scrollTop || 0;
      visit.scroll = Math.max(visit.scroll, Math.min(100, Math.max(0, Math.round((top + window.innerHeight) / height * 100))));
    }
  }
  function deliver(payload, final, attempt) {
    var body = JSON.stringify(payload);
    if ((final || document.hidden) && navigator.sendBeacon) {
      try { if (navigator.sendBeacon(trackURL, body)) return; } catch (_) {}
    }
    if (!window.fetch) return;
    window.fetch(trackURL, { method: "POST", headers: { "Content-Type": "text/plain;charset=UTF-8" },
      body: body, keepalive: true, credentials: "omit" }).then(function (response) {
      if ((response.status === 429 || response.status >= 500) && attempt < 2 && !document.hidden) {
        setTimeout(function () { deliver(payload, final, attempt + 1); }, 1000 * Math.pow(2, attempt));
      }
    }).catch(function () {
      if (attempt < 2 && !document.hidden) {
        setTimeout(function () { deliver(payload, final, attempt + 1); }, 1000 * Math.pow(2, attempt));
      }
    });
  }
  function send(final) {
    if (visit.finished) return;
    updateActive();
    var dwell = Math.min(86400, Math.max(0, Math.floor((performance.now() - visit.start) / 1000)));
    var payload = { visitId: visit.id, sequence: ++visit.sequence, url: visit.url, referrer: visit.referrer,
      dwellTime: dwell, activeTime: Math.min(dwell, Math.floor(visit.activeMs / 1000)), scrollDepth: visit.scroll, final: !!final };
    if (final) visit.finished = true;
    deliver(payload, final, 0);
  }
  function navigate() {
    var next = canonicalURL(window.location.href);
    if (next === visit.url) return;
    var previous = visit.url;
    send(true);
    visit = newVisit(next, previous);
    send(false);
  }
  function heartbeat() {
    if (visit.finished || document.hidden) return;
    if (performance.now() - visit.start >= maxDuration) {
      send(true);
      visit = newVisit(visit.url, visit.referrer);
    }
    updateScroll();
    send(false);
  }

  document.addEventListener("visibilitychange", function () {
    updateActive();
    visit.hidden = document.hidden;
    visit.tick = performance.now();
    if (!document.hidden) visit.lastActivity = visit.tick;
    send(false);
  });
  window.addEventListener("pagehide", function () { send(true); });
  window.addEventListener("pageshow", function (event) {
    if (event.persisted) {
      visit.finished = false;
      visit.hidden = document.hidden;
      visit.tick = performance.now();
      visit.lastActivity = visit.tick;
      heartbeat();
    }
  });
  ["mousedown", "keydown", "touchstart", "click"].forEach(function (name) {
    document.addEventListener(name, markActive, { passive: true });
  });
  window.addEventListener("scroll", function () { markActive(); updateScroll(); }, { passive: true });
  window.addEventListener("load", updateScroll);
  ["pushState", "replaceState"].forEach(function (name) {
    var original = history[name];
    history[name] = function () { var result = original.apply(history, arguments); navigate(); return result; };
  });
  window.addEventListener("popstate", navigate);
  setInterval(heartbeat, 30000);
  updateScroll();
  send(false);
})();
