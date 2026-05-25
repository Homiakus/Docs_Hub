// =============================================================================
// Docs Hub — Command Palette (Cmd/Ctrl+K)
// Obsidian/Raycast-style quick command interface
// =============================================================================

(() => {
  if (typeof window.__docsHubPaletteInstalled !== "undefined") return;
  window.__docsHubPaletteInstalled = true;

  // --- DOM ------------------------------------------------------------------
  let overlay, input, list, activeIndex = -1, items = [];

  function create() {
    overlay = document.createElement("div");
    overlay.className = "cmd-palette-overlay";
    overlay.innerHTML = `<div class="cmd-palette">
      <div class="cmd-palette-header">
        <input class="cmd-palette-input" placeholder="Что сделать? Поиск команд..." autofocus>
        <span class="cmd-palette-hint">Esc — закрыть · ↑↓ — выбрать · Enter — выполнить</span>
      </div>
      <div class="cmd-palette-results"></div>
    </div>`;
    document.body.appendChild(overlay);

    input = overlay.querySelector(".cmd-palette-input");
    list = overlay.querySelector(".cmd-palette-results");

    input.addEventListener("input", filter);
    input.addEventListener("keydown", navigate);
    overlay.addEventListener("click", (e) => { if (e.target === overlay) close(); });
    document.addEventListener("keydown", globalKey);
  }

  // --- Commands -------------------------------------------------------------

  function getCommands() {
    const cmds = [
      { id: "new-note",    label: "Новая статья",        icon: "N", action: () => { location.href = "/edit/new"; } },
      { id: "go-home",     label: "На главную",          icon: "H", action: () => { location.href = "/"; } },
      { id: "search",      label: "Поиск по статьям",    icon: "/", action: () => { location.href = "/?q="; } },
      { id: "import",      label: "Импорт Markdown",     icon: "I", action: () => { location.href = "/admin/import"; } },
      { id: "users",       label: "Пользователи",         icon: "U", action: () => { location.href = "/admin/users"; } },
      { id: "groups",      label: "Группы",               icon: "G", action: () => { location.href = "/admin/groups"; } },
      { id: "backups",     label: "Резервные копии",      icon: "B", action: () => { location.href = "/admin/backups"; } },
      { id: "audit",       label: "Аудит",                icon: "A", action: () => { location.href = "/admin/audit"; } },
      { id: "ribbon",      label: "Настроить ленту",      icon: "R", action: () => { location.href = "/admin/ribbon"; } },
      { id: "logout",      label: "Выйти",                icon: "Q", action: () => { location.href = "/logout"; } },
    ];

    // Dynamically add ribbon articles for quick navigation
    try {
      const ribbonLinks = document.querySelectorAll(".ribbon-note");
      ribbonLinks.forEach((a) => {
        const slug = a.getAttribute("href")?.replace("/a/", "");
        const title = a.querySelector("span")?.textContent || slug;
        if (slug && title && slug !== "all") {
          cmds.push({
            id: "go-" + slug,
            label: "→ " + title,
            icon: "D",
            action: () => { location.href = "/a/" + slug; }
          });
        }
      });
    } catch (_) {}

    return cmds;
  }

  // --- Filter & render ------------------------------------------------------

  function fuzzyMatch(text, query) {
    if (!query) return true;
    text = text.toLowerCase();
    query = query.toLowerCase();
    let qi = 0;
    for (let i = 0; i < text.length && qi < query.length; i++) {
      if (text[i] === query[qi]) qi++;
    }
    return qi === query.length;
  }

  function filter() {
    const q = input.value.trim();
    items = q ? getCommands().filter(c => fuzzyMatch(c.label, q) || fuzzyMatch(c.id, q)) : getCommands();
    activeIndex = items.length > 0 ? 0 : -1;
    render();
  }

  function render() {
    if (items.length === 0) {
      list.innerHTML = `<div class="cmd-palette-empty">Ничего не найдено</div>`;
      return;
    }
    list.innerHTML = items.map((c, i) =>
      `<div class="cmd-palette-item${i === activeIndex ? ' active' : ''}" data-index="${i}">
        <span class="cmd-palette-icon">${c.icon}</span>
        <span class="cmd-palette-label">${escHTML(c.label)}</span>
      </div>`
    ).join("");

    // Click handlers
    list.querySelectorAll(".cmd-palette-item").forEach(el => {
      el.addEventListener("click", () => {
        const idx = parseInt(el.dataset.index);
        if (items[idx]) { close(); items[idx].action(); }
      });
    });
  }

  function escHTML(s) {
    return String(s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'})[m]);
  }

  // --- Navigation -----------------------------------------------------------

  function navigate(e) {
    if (e.key === "Escape") { close(); return; }
    if (e.key === "Enter") {
      e.preventDefault();
      if (items[activeIndex]) { close(); items[activeIndex].action(); }
      return;
    }
    if (e.key === "ArrowDown") { e.preventDefault(); activeIndex = Math.min(activeIndex + 1, items.length - 1); render(); }
    if (e.key === "ArrowUp")   { e.preventDefault(); activeIndex = Math.max(activeIndex - 1, 0); render(); }
  }

  // --- Open / Close ---------------------------------------------------------

  function open() {
    if (!overlay) create();
    overlay.style.display = "flex";
    items = getCommands();
    activeIndex = 0;
    input.value = "";
    render();
    setTimeout(() => input.focus(), 50);
  }

  function close() {
    if (overlay) overlay.style.display = "none";
  }

  function globalKey(e) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      overlay && overlay.style.display === "flex" ? close() : open();
    }
  }

  // --- Init -----------------------------------------------------------------
  create();
  overlay.style.display = "none";
})();
