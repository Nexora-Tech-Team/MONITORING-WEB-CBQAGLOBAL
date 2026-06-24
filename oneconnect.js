const menuGroups = Array.from(document.querySelectorAll(".side-menu__group"));
const mobileQuery = window.matchMedia("(max-width: 760px)");
const desktopViewportMargin = 16;

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

function positionSubmenu(group) {
  const trigger = group.querySelector(".side-menu__item");
  const submenu = group.querySelector(".submenu-panel");
  const sideMenu = document.querySelector(".side-menu");

  if (!trigger || !submenu || !sideMenu) return;

  if (mobileQuery.matches) {
    submenu.style.removeProperty("--submenu-top");
    submenu.style.removeProperty("--submenu-right");
    submenu.style.removeProperty("--submenu-arrow-top");
    submenu.style.removeProperty("max-height");
    return;
  }

  const menuRect = sideMenu.getBoundingClientRect();
  const triggerRect = trigger.getBoundingClientRect();
  const viewportHeight = window.innerHeight;
  const availableHeight = Math.max(280, viewportHeight - desktopViewportMargin * 2);

  submenu.style.maxHeight = `${availableHeight}px`;

  const panelHeight = Math.min(submenu.scrollHeight, availableHeight);
  const idealTop = triggerRect.top + triggerRect.height / 2;
  const minTop = desktopViewportMargin + panelHeight / 2;
  const maxTop = viewportHeight - desktopViewportMargin - panelHeight / 2;
  const top = clamp(idealTop, minTop, Math.max(minTop, maxTop));
  const right = Math.max(0, window.innerWidth - menuRect.left - 1);
  const panelTop = top - panelHeight / 2;
  const arrowTop = clamp(idealTop - panelTop, 24, Math.max(24, panelHeight - 24));

  submenu.style.setProperty("--submenu-top", `${top}px`);
  submenu.style.setProperty("--submenu-right", `${right}px`);
  submenu.style.setProperty("--submenu-arrow-top", `${arrowTop}px`);
}

function positionOpenSubmenus() {
  menuGroups.forEach(function (group) {
    if (group.matches(":hover") || group.matches(":focus-within") || group.classList.contains("is-open")) {
      positionSubmenu(group);
    }
  });
}

function closeMenuGroups(exceptGroup) {
  menuGroups.forEach(function (group) {
    if (group === exceptGroup) return;

    group.classList.remove("is-open");
    const trigger = group.querySelector(".side-menu__item");
    if (trigger) {
      trigger.setAttribute("aria-expanded", "false");
    }
  });
}

menuGroups.forEach(function (group) {
  const trigger = group.querySelector(".side-menu__item");
  const submenu = group.querySelector(".submenu-panel");
  let touchedRecently = false;

  if (!trigger || !submenu) return;

  group.addEventListener("mouseenter", function () {
    positionSubmenu(group);
  });

  group.addEventListener("focusin", function () {
    positionSubmenu(group);
  });

  function toggleSubmenu(event) {
    event.preventDefault();
    const shouldOpen = !group.classList.contains("is-open");
    closeMenuGroups(group);
    group.classList.toggle("is-open", shouldOpen);
    trigger.setAttribute("aria-expanded", String(shouldOpen));
    if (shouldOpen) positionSubmenu(group);
  }

  trigger.addEventListener("click", function (event) {
    if (touchedRecently) {
      event.preventDefault();
      return;
    }

    toggleSubmenu(event);
  });
  trigger.addEventListener("touchstart", function (event) {
    touchedRecently = true;
    toggleSubmenu(event);
    window.setTimeout(function () {
      touchedRecently = false;
    }, 450);
  }, { passive: false });
});

document.addEventListener("click", function (event) {
  if (!mobileQuery.matches) return;
  if (event.target.closest(".side-menu__group")) return;

  closeMenuGroups();
});

document.addEventListener("keydown", function (event) {
  if (event.key === "Escape") closeMenuGroups();
});

if (typeof mobileQuery.addEventListener === "function") {
  mobileQuery.addEventListener("change", function () {
    closeMenuGroups();
    positionOpenSubmenus();
  });
} else if (typeof mobileQuery.addListener === "function") {
  mobileQuery.addListener(function () {
    closeMenuGroups();
    positionOpenSubmenus();
  });
}

window.addEventListener("resize", positionOpenSubmenus);
window.addEventListener("scroll", positionOpenSubmenus, { passive: true });
