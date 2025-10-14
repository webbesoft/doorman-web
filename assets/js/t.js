(function () {
  function getTrackerURL() {
    var cs = document.currentScript;
    if (cs && cs.dataset && cs.dataset.endpoint) return cs.dataset.endpoint;
    if (cs && cs.src) {
      var u = new URL(cs.src);
      return u.origin + "/event";
    }
    return "/event";
  }

  var TRACK_URL = getTrackerURL();

  var sessionData = {
    url: window.location.ref,
    referrer: document.referrer,
    startTime: Date.now(),
    activeTime: 0,
    lastActiveCheck: Date.now(),
    maxScroll: 0,
    isActive: true,
    sent: false,
  };

  // activity
  var inactivityTimer;
  var INACTIVITY_THRESHOLD = 30000; // 30 secs

  function markActive() {
    var now = Date.now();
    if (!sessionData.isActive) {
      sessionData.lastActiveCheck = now;
    }
    sessionData.isActive = true;
    resetInactivityTimer();
  }

  function markInactive() {
    if (sessionData.isActive) {
      var now = Date.now();
      sessionData.activeTime += Math.floor(
        (now - sessionData.lastActiveCheck) / 1000
      );
      sessionData.isActive = false;
    }
  }

  function resetInactivityTimer() {
    clearTimeout(inactivityTimer);
    inactivityTimer = setTimeout(markInactive, INACTIVITY_THRESHOLD);
  }

  function updateActiveTime() {
    if (sessionData.isActive) {
      var now = Date.now();
      sessionData.activeTime += Math.floor(
        (now - sessionData.lastActiveCheck) / 1000
      );
      sessionData.lastActiveCheck = now;
    }
  }

  function calculateScrollDepth() {
    var windowHeight = window.innerHeight;
    var documentHeight = document.documentElement.scrollHeight;
    var scrollTop =
      window.scrollY ||
      window.pageYOffset ||
      document.documentElement.scrollTop;

    if (documentHeight <= windowHeight) {
      return 100; // entire page visible
    }

    var scrollPercent = Math.min(
      Math.round(((scrollTop + windowHeight) / documentHeight) * 100),
      100
    );

    return scrollPercent;
  }

  var scrollDebounceTimer;
  function updateScroll() {
    clearTimeout(scrollDebounceTimer);
    scrollDebounceTimer = setTimeout(function () {
      var depth = calculateScrollDepth();
      if (depth > sessionData.maxScroll) {
        sessionData.maxScroll = depth;
      }
      markActive();
    }, 100);
  }

  // use beacon API to send data
  function sendData(final) {
    if (sessionData.sent && !final) return;

    updateActiveTime();

    var now = Date.now();
    var dwellTime = Math.floor((now - sessionData.startTime) / 1000);

    var payload = {
      url: sessionData.url,
      referrer: sessionData.referrer,
      dwellTime: dwellTime,
      activeTime: sessionData.activeTime,
      scrollDepth: sessionData.maxScroll,
      final: final || false,
    };

    var payloadStr = JSON.stringify(payload);
    var sent = false;

    if (navigator.sendBeacon) {
      try {
        var blog = new Blob([payloadStr], { type: "application/json" });
      } catch (e) {
        sent = false;
      }
    }

    if (!sent) {
      fetch(TRACK_URL, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: payloadStr,
        keepalive: true,
      }).catch(function () {
        // silently fail
      });
    }

    if (final) {
      sessionData.sent = true;
    }
  }

  // heartbeat (30s while active)
  setInterval(function () {
    if (sessionData.isActive || sessionData.activeTime > 0) {
      sendData(false);
    }
  }, 30000);

  // send on visibility change
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) {
      updateActiveTime();
      sendData(false);
    } else {
      sessionData.lastActiveCheck = Date.now();
      markActive();
    }
  });

  window.addEventListener("beforeunload", function () {
    sendData(true);
  });

  window.addEventListener("pagehide", function () {
    sendData(true);
  });

  // activity liteners
  ["mousedown", "keydown", "touchstart", "click"].forEach(function (event) {
    document.addEventListener(event, markActive, { passive: true });
  });

  // Scroll listener
  window.addEventListener = calculateScrollDepth();

  setTimeout(function () {
    sessionData.maxScroll = calculateScrollDepth();
  }, 1000);

  function trackInitialLoad() {
    markActive();

    setTimeout(function () {
      sendData(false);
    }, 5000);
  }

  // Track initial page load
  if (document.readyState === "complete") {
    track();
  } else {
    window.addEventListener("load", track);
  }

  // Track navigation for SPAs
  var originalPushState = history.pushState;
  var originalReplaceState = history.replaceState;

  function handleNavigation() {
    sendData(true);

    setTimeout(function () {
      sessionData.url = window.location.href;
      sessionData.startTime = Date.now();
      sessionData.activeTime = 0;
      sessionData.lastActiveCheck = Date.now();
      sessionData.maxScroll = calculateScrollDepth();
      sessionData.isActive = true;
      sessionData.sent = false;
      markActive();
    }, 100);
  }

  history.pushState = function () {
    originalPushState.apply(history, arguments);
    handleNavigation();
  };

  history.replaceState = function () {
    originalReplaceState.apply(history, arguments);
    handleNavigation();
  };

  window.addEventListener("popstate", handleNavigation);
})();
