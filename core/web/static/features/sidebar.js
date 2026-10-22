// The sidebar lives outside the swapped .app-content, so its active link goes
// stale after a soft navigation. Prefer what the server rendered for the new
// page; fall back to matching the current path.
export function refreshSidebarActive(newDoc) {
  const links = Array.from(document.querySelectorAll(".sidebar-link"));
  if (!links.length) {
    return;
  }

  const source = newDoc || document;
  const serverActive = source.querySelector(".sidebar-link.is-active");
  let activeLink = null;

  if (serverActive) {
    const serverHref = serverActive.getAttribute("href");
    activeLink = links.find(function(link) {
      return link.getAttribute("href") === serverHref;
    }) || null;
  }

  if (!activeLink) {
    const normalizePath = function(pathname) {
      if (!pathname) {
        return "/";
      }
      const trimmed = pathname.replace(/\/$/, "");
      return trimmed === "" ? "/" : trimmed;
    };
    const currentPath = normalizePath(window.location.pathname);
    let activeLength = -1;

    links.forEach(function(link) {
      const linkPath = normalizePath(new URL(link.getAttribute("href"), window.location.origin).pathname);
      const matches = currentPath === linkPath || (linkPath !== "/" && currentPath.indexOf(linkPath + "/") === 0);
      if (matches && linkPath.length > activeLength) {
        activeLink = link;
        activeLength = linkPath.length;
      }
    });
  }

  links.forEach(function(link) {
    link.classList.remove("is-active");
    link.removeAttribute("aria-current");
  });
  if (activeLink) {
    activeLink.classList.add("is-active");
    activeLink.setAttribute("aria-current", "page");
  }
}

export function initSidebar() {
  const body = document.body;
  const toggle = document.getElementById("sidebar-toggle");
  const sidebar = document.getElementById("dashboard-sidebar");
  const scrim = document.querySelector(".sidebar-scrim");
  const closeBtn = document.querySelector(".sidebar-close");

  if (!toggle || !sidebar || !scrim || toggle.hasAttribute("data-sidebar-bound")) {
    return;
  }
  toggle.setAttribute("data-sidebar-bound", "true");

  const isDesktop = function() {
    return window.innerWidth > 960;
  };

  refreshSidebarActive();

  const setState = function(isOpen) {
    if (isDesktop()) {
      body.classList.toggle("sidebar-collapsed", !isOpen);
      body.classList.remove("sidebar-open");
    } else {
      body.classList.toggle("sidebar-open", isOpen);
      body.classList.remove("sidebar-collapsed");
    }
    toggle.setAttribute("aria-expanded", isOpen ? "true" : "false");
    sidebar.setAttribute("aria-hidden", isOpen ? "false" : "true");
  };

  const closeSidebar = function() {
    setState(false);
  };

  const syncForViewport = function() {
    if (isDesktop()) {
      body.classList.remove("sidebar-open");
      toggle.setAttribute("aria-expanded", body.classList.contains("sidebar-collapsed") ? "false" : "true");
      sidebar.setAttribute("aria-hidden", body.classList.contains("sidebar-collapsed") ? "true" : "false");
    } else {
      body.classList.remove("sidebar-collapsed");
      toggle.setAttribute("aria-expanded", body.classList.contains("sidebar-open") ? "true" : "false");
      sidebar.setAttribute("aria-hidden", body.classList.contains("sidebar-open") ? "false" : "true");
    }
  };

  toggle.addEventListener("click", function() {
    const isOpen = isDesktop() ? !body.classList.contains("sidebar-collapsed") : body.classList.contains("sidebar-open");
    setState(!isOpen);
  });

  scrim.addEventListener("click", closeSidebar);

  if (closeBtn) {
    closeBtn.addEventListener("click", closeSidebar);
  }

  sidebar.addEventListener("click", function(event) {
    const target = event.target;
    if (!target || !target.closest) {
      return;
    }
    const link = target.closest("a");
    if (link && !isDesktop()) {
      closeSidebar();
    }
  });

  document.addEventListener("keydown", function(event) {
    if (event.key === "Escape") {
      closeSidebar();
    }
  });

  window.addEventListener("resize", syncForViewport);
  syncForViewport();
}
