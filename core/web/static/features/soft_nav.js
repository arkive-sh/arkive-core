// Soft navigation for AuthLayout pages (.app-shell + .app-content).
// Clicking an internal link fetches the next page, swaps only .app-content,
// re-runs that page's scripts and updates the URL via history.pushState.
// Anything unexpected falls back to a normal full page load.

const PAGE_MODULE_TYPE = "module";
const APP_SCRIPT_SRC = "/static/app.js";

let started = false;
let navToken = 0;
let activeFetchController = null;
let renderedLocation = window.location.pathname + window.location.search;

const loadedModuleSources = new Set();
let activeModules = [];

const beforeSwapHooks = [];
const afterSwapHooks = [];

export function onBeforeSwap(hook) {
  if (typeof hook === "function") {
    beforeSwapHooks.push(hook);
  }
}

export function onAfterSwap(hook) {
  if (typeof hook === "function") {
    afterSwapHooks.push(hook);
  }
}

export function initSoftNav() {
  if (started) {
    return;
  }
  started = true;

  if ("scrollRestoration" in history) {
    history.scrollRestoration = "manual";
  }

  recordCurrentPageModules();
  document.addEventListener("click", onDocumentClick);
  window.addEventListener("popstate", onPopState);
}

function runHooks(hooks, argument) {
  hooks.forEach(function(hook) {
    try {
      hook(argument);
    } catch (_) {
    }
  });
}

function fullLoad(url) {
  window.location.assign(url.href);
}

function isModifiedClick(event) {
  return event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
}

function parseURL(href) {
  try {
    return new URL(href, window.location.href);
  } catch (_) {
    return null;
  }
}

function isSoftNavTarget(anchor, url) {
  if (!anchor || !url) {
    return false;
  }
  if (anchor.closest("form")) {
    return false;
  }
  const href = anchor.getAttribute("href") || "";
  if (!href || href.charAt(0) === "#") {
    return false;
  }
  if (anchor.hasAttribute("download")) {
    return false;
  }
  const target = anchor.getAttribute("target");
  if (target && target !== "_self") {
    return false;
  }
  if (url.origin !== window.location.origin) {
    return false;
  }
  if (url.pathname === window.location.pathname && url.search === window.location.search) {
    return false;
  }
  return true;
}

function onDocumentClick(event) {
  if (event.defaultPrevented || isModifiedClick(event)) {
    return;
  }
  const anchor = event.target && event.target.closest ? event.target.closest("a[href]") : null;
  if (!anchor) {
    return;
  }
  const url = parseURL(anchor.href);
  if (!isSoftNavTarget(anchor, url)) {
    return;
  }
  event.preventDefault();
  navigate(url, { push: true });
}

function onPopState() {
  const url = parseURL(window.location.href);
  if (!url) {
    return;
  }
  // Every traversal invalidates in-flight navigations: an older fetch's swap
  // must never land after the URL has already moved on. Without this, rapid
  // back/forward could leave DOM and URL on different pages.
  navToken++;
  if (activeFetchController) {
    activeFetchController.abort();
    activeFetchController = null;
  }
  const locationKey = url.pathname + url.search;
  if (locationKey === renderedLocation) {
    return;
  }
  navigate(url, { push: false });
}

export async function navigate(url, options) {
  if (typeof url === "string") {
    url = parseURL(url);
    if (!url) {
      return;
    }
  }
  const token = ++navToken;
  if (activeFetchController) {
    activeFetchController.abort();
  }
  const controller = new AbortController();
  activeFetchController = controller;

  if (options.push) {
    saveScrollIntoCurrentEntry();
  }

  let response = null;
  try {
    response = await fetch(url.pathname + url.search, {
      signal: controller.signal,
      credentials: "same-origin",
      headers: {
        "Accept": "text/html",
        "X-Requested-With": "ArkiveSoftNav",
      },
    });
  } catch (_) {
    if (token === navToken) {
      fullLoad(url);
    }
    return;
  }

  if (token !== navToken) {
    return;
  }

  const contentType = (response.headers.get("content-type") || "").toLowerCase();
  if (!response.ok || contentType.indexOf("text/html") === -1) {
    fullLoad(url);
    return;
  }

  const finalURL = parseURL(response.url);
  if (!finalURL || finalURL.origin !== window.location.origin) {
    fullLoad(finalURL || url);
    return;
  }
  // The server redirected somewhere else (login, lock, another page): full load.
  if (finalURL.pathname !== url.pathname) {
    fullLoad(finalURL);
    return;
  }

  let doc = null;
  try {
    const html = await response.text();
    if (token !== navToken) {
      return;
    }
    doc = new DOMParser().parseFromString(html, "text/html");
  } catch (_) {
    fullLoad(url);
    return;
  }

  const nextContent = doc.querySelector(".app-content");
  if (!nextContent) {
    fullLoad(url);
    return;
  }

  await swapPage(doc, nextContent, url, options, token);
}

// Classic scripts (e.g. dashboard.js) cannot import modules; expose the same
// soft navigation as a global entry point.
window.softNavigate = function (href) {
  return navigate(href, { push: true });
};

async function swapPage(doc, nextContent, url, options, token) {
  syncStyles(doc);

  runHooks(beforeSwapHooks);
  await cleanupActiveModules();

  // A newer navigation started while we were awaiting: let it own the DOM.
  if (token !== navToken) {
    return;
  }

  document.title = doc.title || document.title;
  const appContent = document.querySelector(".app-content");
  // The dashboard adopts the upload tray into its content; pull it back to the
  // shell so replaceChildren cannot destroy it (and any running upload).
  const uploadTray = document.querySelector(".app-upload-tray");
  if (uploadTray && appContent.contains(uploadTray)) {
    appContent.after(uploadTray);
  }
  const nodes = Array.from(nextContent.childNodes).map(function(node) {
    return document.importNode(node, true);
  });
  appContent.replaceChildren.apply(appContent, nodes);

  renderedLocation = url.pathname + url.search;
  // Keys the upload-tray visibility rule (auth_layout.css): only the dashboard
  // shows it; every other page hides the same still-mounted node.
  document.body.setAttribute("data-page", url.pathname);
  if (options.push) {
    history.pushState({ arkiveSoftNav: true, scrollX: 0, scrollY: 0 }, "", url.pathname + url.search + url.hash);
  }

  await loadPageScripts(doc, url);

  runHooks(afterSwapHooks, doc);

  restoreScroll(options, url);
}

function restoreScroll(options, url) {
  if (!options.push) {
    const state = history.state || {};
    if (typeof state.scrollY === "number") {
      window.scrollTo(state.scrollX || 0, state.scrollY);
      return;
    }
  }
  scrollToHash(url.hash);
}

function scrollToHash(hash) {
  if (!hash || hash === "#") {
    window.scrollTo(0, 0);
    return;
  }
  let target = null;
  try {
    target = document.querySelector(hash);
  } catch (_) {
    target = null;
  }
  if (target && typeof target.scrollIntoView === "function") {
    target.scrollIntoView({ block: "start" });
    return;
  }
  window.scrollTo(0, 0);
}

function saveScrollIntoCurrentEntry() {
  const state = Object.assign({}, history.state || {}, {
    arkiveSoftNav: true,
    scrollX: window.scrollX,
    scrollY: window.scrollY,
  });
  history.replaceState(state, "");
}

function syncStyles(doc) {
  const currentLinks = Array.from(document.head.querySelectorAll("link[rel='stylesheet']"));
  const currentHrefs = new Set(currentLinks.map(function(link) {
    return link.getAttribute("href");
  }));
  const nextHrefs = Array.from(doc.head.querySelectorAll("link[rel='stylesheet']")).map(function(link) {
    return link.getAttribute("href");
  });
  const nextHrefsSet = new Set(nextHrefs);

  // Add the new page's styles before dropping the old ones.
  nextHrefs.forEach(function(href) {
    if (currentHrefs.has(href)) {
      return;
    }
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    document.head.appendChild(link);
  });

  currentLinks.forEach(function(link) {
    if (!nextHrefsSet.has(link.getAttribute("href"))) {
      link.parentNode.removeChild(link);
    }
  });
}

function pageScriptsFrom(doc) {
  return Array.from(doc.head.querySelectorAll("script[src]")).filter(function(script) {
    const src = script.getAttribute("src");
    return src && src !== APP_SCRIPT_SRC;
  });
}

// Page modules that are also import dependencies of app.js (e.g. files.js via
// context_menu.js) get evaluated on pages that do not list them as page
// scripts. They call this at the top level so the first soft swap still
// re-initializes them against the swapped DOM.
export function markModuleLoaded(src) {
  if (!src || loadedModuleSources.has(src)) {
    return;
  }
  loadedModuleSources.add(src);
  activeModules.push({ src: src, mod: null });
}

function recordCurrentPageModules() {
  pageScriptsFrom(document).forEach(function(script) {
    const src = script.getAttribute("src");
    if ((script.getAttribute("type") || "").toLowerCase() !== PAGE_MODULE_TYPE) {
      return;
    }
    // The browser already evaluated these on the initial page load.
    markModuleLoaded(src);
  });
}

async function cleanupActiveModules() {
  const modules = activeModules;
  activeModules = [];
  for (const entry of modules) {
    try {
      const mod = entry.mod || await import(entry.src);
      if (mod && typeof mod.cleanupPage === "function") {
        mod.cleanupPage();
      }
    } catch (_) {
    }
  }
}

async function loadPageScripts(doc, url) {
  const scripts = pageScriptsFrom(doc);
  for (const script of scripts) {
    const src = script.getAttribute("src");
    const isModule = (script.getAttribute("type") || "").toLowerCase() === PAGE_MODULE_TYPE;

    if (!isModule) {
      appendClassicScript(src);
      continue;
    }

    // Decide before importing: a module first evaluated by this import
    // self-initializes against the already-swapped DOM, but a module that was
    // evaluated earlier (page script of a previous page, or an app.js import
    // dependency) initialized against a stale DOM and needs re-init below.
    const alreadyLoaded = loadedModuleSources.has(src);
    loadedModuleSources.add(src);

    let mod = null;
    try {
      mod = await import(src);
    } catch (_) {
      fullLoad(url);
      return;
    }

    activeModules.push({ src: src, mod: mod });

    if (!alreadyLoaded) {
      // First import: the module initializes itself at the top level.
      continue;
    }

    try {
      if (typeof mod.cleanupPage === "function") {
        mod.cleanupPage();
      }
      if (typeof mod.initPage === "function") {
        await mod.initPage();
      }
    } catch (_) {
    }
  }
}

function appendClassicScript(src) {
  // Vendor bundles that define a global are skipped when already present.
  if (src.indexOf("plyr.polyfilled.js") !== -1 && window.Plyr) {
    return;
  }
  const script = document.createElement("script");
  script.src = src;
  script.defer = true;
  document.head.appendChild(script);
}
