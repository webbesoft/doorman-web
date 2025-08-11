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

  // GDPR compliant tracker - no cookies, no personal data
  function track() {
    const userAgent = navigator.userAgent;
    fetch(TRACK_URL, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        url: window.location.href,
        referrer: document.referrer,
        agent: navigator.userAgent,
      }),
      keepalive: true,
    }).catch(function () {
      // Silently fail
    });
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

  history.pushState = function () {
    originalPushState.apply(history, arguments);
    setTimeout(track, 100);
  };

  history.replaceState = function () {
    originalReplaceState.apply(history, arguments);
    setTimeout(track, 100);
  };

  window.addEventListener("popstate", function () {
    setTimeout(track, 100);
  });
})();
