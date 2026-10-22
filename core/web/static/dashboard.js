(function() {
  // Adopt the shell upload panel into the dashboard layout (summary → upload →
  // recent files). It is the same DOM node, so listeners and any in-progress
  // upload survive; soft_nav.js moves it back to the shell before swaps.
  var uploadSlot = document.getElementById("dashboard-upload-slot");
  var uploadTray = document.querySelector(".app-upload-tray");
  if (uploadSlot && uploadTray && uploadTray.parentNode !== uploadSlot) {
    uploadSlot.appendChild(uploadTray);
    if (window.location.hash === "#upload-panel" && uploadTray.scrollIntoView) {
      uploadTray.scrollIntoView();
    }
  }

  const rows = document.querySelectorAll("[data-activity-open]");
  if (!rows.length) {
    return;
  }

  function openRow(row) {
    const href = String(row.getAttribute("data-activity-open") || "");
    if (!href) {
      return;
    }
    window.softNavigate(href);
  }

  rows.forEach(function(row) {
    if (row.hasAttribute("data-activity-open-bound")) {
      return;
    }
    row.setAttribute("data-activity-open-bound", "true");

    row.addEventListener("click", function(event) {
      if (event.target.closest("a, button, input, label")) {
        return;
      }
      openRow(row);
    });

    row.addEventListener("keydown", function(event) {
      if (event.key !== "Enter" && event.key !== " " && event.key !== "Spacebar") {
        return;
      }
      event.preventDefault();
      openRow(row);
    });
  });
})();
