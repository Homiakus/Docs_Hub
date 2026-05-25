// =============================================================================
// Docs Hub — Toast UI Editor
// Knowledge-base article editor with WYSIWYG + Markdown
// =============================================================================

// --- DOM references -----------------------------------------------------------

const md = document.getElementById("md");
const host = document.getElementById("toastEditor");
const shell = document.querySelector(".live-editor-shell");
const previewStatus = document.getElementById("previewStatus");
const editorForm = document.getElementById("editorForm");
const csrf =
  document.querySelector('#editorForm input[name="csrf"]')?.value || "";
const titleInput = document.querySelector('#editorForm input[name="title"]');
const slugInput = document.querySelector('#editorForm input[name="slug"]');
let articleId = shell?.dataset.articleId || "";
let toastEditor = null;

// --- State -------------------------------------------------------------------

let lastSavedMarkdown = ""; // content last persisted via manual save
let lastAutoSavedMarkdown = ""; // content last persisted via auto-save
let wikiDropdown = null; // autocomplete dropdown element
let wikiMatchIndex = -1; // currently highlighted item in dropdown
let wikiMatches = []; // current matching articles

// =============================================================================
//  Utility helpers
// =============================================================================

/** Escape HTML special characters. */
function esc(s) {
  return String(s || "").replace(
    /[&<>"']/g,
    (m) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        m
      ],
  );
}

/** Produce a URL-safe slug from arbitrary text. */
function slug(s) {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^\p{L}\p{N}]+/gu, "-")
    .replace(/^-+|-+$/g, "");
}

/** Sanitise a filename for use as a markdown label. */
function mdLabel(s) {
  s = String(s || "file")
    .replace(/[\r\n\[\]()]/g, " ")
    .trim();
  return s || "file";
}

/** Clean a raw URL for markdown usage (strip trailing parens / spaces). */
function mdURL(s) {
  return String(s || "")
    .trim()
    .replace(/[)\s]+/g, "");
}

/** Build a markdown snippet from uploaded-media response data. */
function mediaMarkdown(data) {
  const mime = data.mime || "";
  if (data.markdown) return data.markdown;
  const url = mdURL(data.embed_url || data.url || "");
  const name = mdLabel(data.name || "file");
  if (mime.startsWith("image/")) return "![" + name + "|100%](" + url + ")";
  if (mime.startsWith("video/")) return "![" + name + "|760](" + url + ")";
  if (mime.startsWith("audio/")) return "![" + name + "](" + url + ")";
  if (mime === "application/pdf" || mime.startsWith("model/")) {
    return "![" + name + "|100%](" + url + ")";
  }
  return "[" + name + "](" + url + ")";
}

// =============================================================================
//  Status & sync
// =============================================================================

function setPreviewStatus(text, state) {
  if (!previewStatus) return;
  previewStatus.textContent = text;
  previewStatus.dataset.state = state || "";
}

function syncMarkdown() {
  if (md && toastEditor) {
    md.value = toastEditor.getMarkdown();
  }
}

// =============================================================================
//  Dynamic asset loading
// =============================================================================

function loadStyle(href) {
  if (document.querySelector('link[href="' + href + '"]')) return;
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.href = href;
  document.head.appendChild(link);
}

function loadScript(src) {
  return new Promise((resolve, reject) => {
    if (document.querySelector('script[src="' + src + '"]')) {
      resolve();
      return;
    }
    const s = document.createElement("script");
    s.src = src;
    s.onload = resolve;
    s.onerror = reject;
    document.head.appendChild(s);
  });
}

// =============================================================================
//  Markdown insertion helpers
// =============================================================================

function normalizeInsert(markdown) {
  markdown = String(markdown || "");
  return /\n$/.test(markdown) ? markdown : markdown + "\n";
}

/** Insert a markdown snippet at the current cursor position. */
function insertMarkdown(markdown) {
  if (!toastEditor) return;
  markdown = normalizeInsert(markdown);
  toastEditor.focus();
  try {
    const sel = toastEditor.getSelection && toastEditor.getSelection();
    if (typeof toastEditor.replaceSelection === "function") {
      toastEditor.replaceSelection(markdown, sel && sel[0], sel && sel[1]);
    } else if (typeof toastEditor.insertText === "function") {
      toastEditor.insertText(markdown);
    } else {
      const current = toastEditor.getMarkdown();
      toastEditor.setMarkdown(
        current + (current.endsWith("\n") ? "" : "\n") + markdown,
        false,
      );
    }
  } catch (_e) {
    const current = toastEditor.getMarkdown();
    toastEditor.setMarkdown(
      current + (current.endsWith("\n") ? "" : "\n") + markdown,
      false,
    );
  }
  syncMarkdown();
  setPreviewStatus("изменено", "ok");
}

function insertUploadedMedia(data) {
  if (!toastEditor || !data) return;
  toastEditor.focus();
  insertMarkdown(mediaMarkdown(data));
}

/** Prompt for an article name and insert a wiki-link. */
function insertWikiLink() {
  const name = prompt("Статья для wiki-ссылки", "");
  if (!name) return;
  insertMarkdown("[[" + slug(name) + "|" + name + "]]");
}

/** Prompt for a YouTube / video URL and insert a markdown link. */
function insertVideoLink() {
  const raw = prompt("YouTube или video URL", "");
  const url = mdURL(raw);
  if (!url) return;
  const title = mdLabel(prompt("Подпись", "Видео") || "Видео");
  insertMarkdown("[" + title + "](" + url + ")");
}

// =============================================================================
//  Attachments — HTML preview (used outside the editor)
// =============================================================================

function htmlForAttachment(x) {
  if (x.html) return x.html;
  const name = esc(x.name || "file");
  const url = esc(x.url || "#");
  const m = x.mime || "";
  if (m.startsWith("image/"))
    return '<img class="md-img" alt="' + name + '" src="' + url + '">';
  if (m.startsWith("audio/"))
    return '<audio class="md-media" controls src="' + url + '"></audio>';
  if (m.startsWith("video/"))
    return '<video class="md-media" controls src="' + url + '"></video>';
  if (m === "application/pdf")
    return '<div class="md-pdf" data-pdf-src="' + url + '"><a href="' + url + '">' + name + "</a></div>";
  if (m.startsWith("model/"))
    return '<div class="md-model"><model-viewer src="' + url + '" alt="' + name + '" camera-controls></model-viewer></div>';
  return '<a href="' + url + '">' + name + "</a>";
}

// =============================================================================
//  Article lifecycle (draft creation, uploads)
// =============================================================================

/** Ensure we have a real article ID. Creates a server-side draft if needed. */
async function ensureArticle() {
  if (articleId) return articleId;
  setPreviewStatus("черновик", "loading");
  const res = await fetch("/admin/draft", {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
    },
    body: new URLSearchParams({ csrf }),
  });
  if (!res.ok) throw new Error("draft " + res.status);
  const data = await res.json();
  articleId = data.id;
  shell.dataset.articleId = articleId;
  const idInput = document.querySelector('#editorForm input[name="id"]');
  if (idInput) idInput.value = articleId;
  if (data.edit_url) history.replaceState(null, "", data.edit_url);
  return articleId;
}

/** Upload a single file blob and return the server JSON response. */
async function uploadOne(file) {
  await ensureArticle();
  const fd = new FormData();
  fd.append("csrf", csrf);
  fd.append("article_id", articleId);
  fd.append("file", file);
  const res = await fetch("/attachments/drop", { method: "POST", body: fd });
  if (!res.ok) throw new Error("upload " + res.status);
  return await res.json();
}

/** True when every file in the list is an image. */
function allImages(files) {
  return Array.from(files || []).every((f) =>
    String(f.type || "").startsWith("image/"),
  );
}

/** Upload every file and insert its markdown. */
async function uploadFiles(files) {
  if (!files || !files.length) return;
  try {
    setPreviewStatus("загрузка", "loading");
    for (const file of files) {
      const data = await uploadOne(file);
      insertUploadedMedia(data);
    }
    setPreviewStatus("live", "ok");
  } catch (e) {
    setPreviewStatus("ошибка загрузки", "fallback");
    console.warn(e);
  }
}

// =============================================================================
//  Drag & drop / paste
// =============================================================================

function bindDropAndPaste() {
  const root = host.querySelector(".toastui-editor-defaultUI") || host;

  root.addEventListener(
    "dragover",
    (e) => {
      if (e.dataTransfer?.files?.length) {
        shell.classList.add("dragging");
        if (!allImages(e.dataTransfer.files)) e.preventDefault();
      }
    },
    true,
  );

  root.addEventListener(
    "dragleave",
    () => shell.classList.remove("dragging"),
    true,
  );

  root.addEventListener(
    "drop",
    (e) => {
      const files = e.dataTransfer?.files;
      if (files && files.length && !allImages(files)) {
        e.preventDefault();
        e.stopPropagation();
        shell.classList.remove("dragging");
        uploadFiles(files);
      }
    },
    true,
  );

  root.addEventListener(
    "paste",
    (e) => {
      const files = e.clipboardData?.files;
      if (files && files.length && !allImages(files)) {
        e.preventDefault();
        e.stopPropagation();
        uploadFiles(files);
      }
    },
    true,
  );
}

// =============================================================================
//  Auto-save
// =============================================================================

/** Silently persist the current editor content via POST /save. */
async function autoSave() {
  if (!editorForm || !md) return;
  const current = md.value;
  if (current === lastAutoSavedMarkdown) return; // nothing changed

  setPreviewStatus("автосохранение…", "loading");

  const fd = new FormData(editorForm);
  try {
    const res = await fetch("/save", { method: "POST", body: fd });
    if (res.ok || res.redirected) {
      lastAutoSavedMarkdown = current;
      lastSavedMarkdown = current;
      setPreviewStatus("сохранено", "ok");
    } else {
      throw new Error("save " + res.status);
    }
  } catch (e) {
    console.warn("auto-save failed", e);
    setPreviewStatus("ошибка автосохранения", "fallback");
  }
}

function startAutoSave() {
  // Run every 60 s; skip first tick so the page has time to initialise.
  setTimeout(() => {
    setInterval(autoSave, 60_000);
  }, 5_000);
}

// =============================================================================
//  Unsaved-changes warning
// =============================================================================

function hasUnsavedChanges() {
  return md && md.value !== lastSavedMarkdown;
}

window.addEventListener("beforeunload", (e) => {
  if (hasUnsavedChanges()) {
    e.preventDefault();
    // Modern browsers require returnValue to be set (even if they ignore the string).
    e.returnValue = "Есть несохранённые изменения.";
    return e.returnValue;
  }
});

// =============================================================================
//  Wiki-link autocomplete
// =============================================================================

/** Collect article {slug, title} from the sidebar ribbon links. */
function getRibbonArticles() {
  const links = document.querySelectorAll(".ribbon-note");
  return Array.from(links)
    .map((a) => ({
      slug: (a.getAttribute("href") || "").replace("/a/", ""),
      title:
        a.getAttribute("title") || a.querySelector("span")?.textContent || "",
    }))
    .filter((x) => x.slug && x.title);
}

/** Inject a minimal stylesheet for wiki-autocomplete & fallback editor. */
function injectEditorStyles() {
  if (document.getElementById("editor-extra-styles")) return;
  const style = document.createElement("style");
  style.id = "editor-extra-styles";
  style.textContent = `.wiki-ac-item { padding: 8px 12px; cursor: pointer; color: var(--text, #ccc); border-bottom: 1px solid var(--border, #333); }
.wiki-ac-item:last-child { border-bottom: none; }
.wiki-ac-item.active, .wiki-ac-item:hover { background: var(--accent, #3a3aff); color: #fff; }
.wiki-ac-slug { opacity: 0.55; font-size: 0.85em; margin-left: 8px; }
.editor-fallback-notice { background: var(--warn-bg, #3a2e00); color: var(--warn-text, #ffb); padding: 12px 16px; border-radius: 6px; margin-bottom: 12px; font-size: 14px; }
.fallback-editor { width: 100%; min-height: calc(100vh - 380px); background: var(--code-bg, #111); color: var(--text, #ccc); border: 1px solid var(--border, #444); border-radius: 6px; padding: 12px; font-family: monospace; font-size: 14px; resize: vertical; }`;
  document.head.appendChild(style);
}

/** Create the floating autocomplete dropdown (once). */
function createWikiDropdown() {
  if (wikiDropdown) return;
  injectEditorStyles();
  wikiDropdown = document.createElement("div");
  wikiDropdown.className = "wiki-autocomplete";
  wikiDropdown.setAttribute("role", "listbox");
  // Base inline styles — override with .wiki-autocomplete CSS if available.
  Object.assign(wikiDropdown.style, {
    display: "none",
    position: "absolute",
    zIndex: "1000",
    background: "var(--card-bg, #1e1e2e)",
    border: "1px solid var(--border, #444)",
    borderRadius: "6px",
    maxHeight: "280px",
    overflowY: "auto",
    minWidth: "260px",
    boxShadow: "0 8px 24px rgba(0,0,0,0.45)",
    fontFamily: "inherit",
    fontSize: "13px",
  });
  document.body.appendChild(wikiDropdown);
}

/** Build dropdown HTML, position it, bind click / keyboard handlers. */
function showWikiDropdown(items) {
  createWikiDropdown();
  wikiMatches = items;
  wikiMatchIndex = 0;

  wikiDropdown.innerHTML = items
    .map(
      (item, i) =>
        '<div class="wiki-ac-item' +
        (i === 0 ? " active" : "") +
        '" role="option" data-slug="' +
        esc(item.slug) +
        '" data-title="' +
        esc(item.title) +
        '" data-index="' +
        i +
        '">' +
        esc(item.title) +
        ' <span class="wiki-ac-slug">' +
        esc(item.slug) +
        "</span>" +
        "</div>",
    )
    .join("");

  // Position below the editor host.
  const rect = host.getBoundingClientRect();
  wikiDropdown.style.top = rect.top + window.scrollY + 10 + "px";
  wikiDropdown.style.left = rect.left + window.scrollX + "px";
  wikiDropdown.style.display = "block";

  // Click selection.
  wikiDropdown.querySelectorAll(".wiki-ac-item").forEach((el) => {
    el.addEventListener("mousedown", (e) => {
      e.preventDefault();
      selectWikiItem(parseInt(el.dataset.index, 10));
    });
  });
}

function hideWikiDropdown() {
  if (wikiDropdown) wikiDropdown.style.display = "none";
  wikiMatches = [];
  wikiMatchIndex = -1;
}

/** Called when the user picks a wiki-link from the dropdown. */
function selectWikiItem(index) {
  if (index < 0 || index >= wikiMatches.length) return;
  const item = wikiMatches[index];
  insertMarkdown("[[" + item.slug + "|" + item.title + "]]");
  hideWikiDropdown();
  if (toastEditor) toastEditor.focus();
}

/** Arrow-key / Enter / Escape navigation inside the dropdown. */
function handleWikiKeydown(e) {
  if (!wikiDropdown || wikiDropdown.style.display === "none") return;

  if (e.key === "ArrowDown") {
    e.preventDefault();
    wikiMatchIndex = Math.min(wikiMatchIndex + 1, wikiMatches.length - 1);
    highlightWikiItem(wikiMatchIndex);
    return;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    wikiMatchIndex = Math.max(wikiMatchIndex - 1, 0);
    highlightWikiItem(wikiMatchIndex);
    return;
  }
  if (e.key === "Enter") {
    e.preventDefault();
    selectWikiItem(wikiMatchIndex);
    return;
  }
  if (e.key === "Escape") {
    e.preventDefault();
    hideWikiDropdown();
    if (toastEditor) toastEditor.focus();
    return;
  }
}

function highlightWikiItem(index) {
  if (!wikiDropdown) return;
  wikiDropdown.querySelectorAll(".wiki-ac-item").forEach((el, i) => {
    el.classList.toggle("active", i === index);
  });
}

/** Check whether the user has typed an unclosed [[ and show suggestions. */
function checkWikiAutocomplete() {
  if (!toastEditor) return;

  let mdText = "";
  try {
    mdText = toastEditor.getMarkdown();
  } catch (_) {
    return;
  }

  const lastOpen = mdText.lastIndexOf("[[");
  if (lastOpen === -1) {
    hideWikiDropdown();
    return;
  }

  const afterOpen = mdText.substring(lastOpen + 2);

  // Already closed or on a different line → bail.
  const closeIdx = afterOpen.indexOf("]]");
  const newlineIdx = afterOpen.indexOf("\n");
  if (closeIdx !== -1 && (newlineIdx === -1 || closeIdx < newlineIdx)) {
    hideWikiDropdown();
    return;
  }

  const query =
    newlineIdx === -1 ? afterOpen : afterOpen.substring(0, newlineIdx);
  const articles = getRibbonArticles();

  const matches = articles.filter(
    (a) =>
      a.title.toLowerCase().includes(query.toLowerCase()) ||
      a.slug.toLowerCase().includes(query.toLowerCase()),
  );

  if (matches.length > 0 && query.length <= 80) {
    showWikiDropdown(matches);
  } else {
    hideWikiDropdown();
  }
}

// =============================================================================
//  Article slug auto-fill
// =============================================================================

let lastTitleForSlug = "";

/** When the title input changes, auto-fill the slug if it looks auto-generated. */
function autoFillSlug() {
  if (!titleInput || !slugInput) return;
  const title = titleInput.value.trim();
  if (!title) return;

  const currentSlug = slugInput.value.trim();
  const expectedSlug = slug(lastTitleForSlug);

  // Fill only when slug is empty OR matches what the previous title would produce.
  if (!currentSlug || currentSlug === expectedSlug) {
    slugInput.value = slug(title);
  }

  lastTitleForSlug = title;
}

// =============================================================================
//  Editor initialisation
// =============================================================================

async function initToastEditor() {
  if (!host || !md) return;

  // --- Load styles -----------------------------------------------------------

  loadStyle("https://uicdn.toast.com/editor/latest/toastui-editor.min.css");
  loadStyle(
    "https://uicdn.toast.com/editor/latest/theme/toastui-editor-dark.min.css",
  );
  loadStyle(
    "https://uicdn.toast.com/editor-plugin-table-merged-cell/latest/toastui-editor-plugin-table-merged-cell.min.css",
  );

  // --- Load scripts (with offline fallback) ----------------------------------

  try {
    await loadScript(
      "https://uicdn.toast.com/editor/latest/toastui-editor-all.min.js",
    );
    await loadScript(
      "https://uicdn.toast.com/editor-plugin-table-merged-cell/latest/toastui-editor-plugin-table-merged-cell.min.js",
    );
  } catch (_e) {
    // Toast UI Editor couldn't load — show a clear message and plain textarea.
    host.innerHTML =
      '<div class="editor-fallback-notice">' +
      "Редактор не загрузился — проверьте подключение к интернету. " +
      "Можно редактировать в текстовом поле." +
      "</div>" +
      '<textarea id="fallbackEditor" class="fallback-editor"></textarea>';

    const fb = document.getElementById("fallbackEditor");
    fb.value = md.value;
    fb.addEventListener("input", () => {
      md.value = fb.value;
    });
    setPreviewStatus("offline", "fallback");
    lastSavedMarkdown = md.value;
    lastAutoSavedMarkdown = md.value;
    return;
  }

  // --- Instantiate editor ----------------------------------------------------

  const Editor = toastui.Editor;
  const tableMergedCell = Editor.plugin && Editor.plugin.tableMergedCell;

  toastEditor = new Editor({
    el: host,
    height: "calc(100vh - 310px)",
    initialValue: md.value || "",
    initialEditType: "wysiwyg",
    previewStyle: "tab",
    hideModeSwitch: true,
    theme: "dark",
    usageStatistics: false,
    plugins: tableMergedCell ? [tableMergedCell] : [],
    toolbarItems: [
      ["heading", "bold", "italic", "strike"],
      ["hr", "quote"],
      ["ul", "ol", "task"],
      ["table", "image", "link"],
      ["code", "codeblock"],
    ],
    hooks: {
      addImageBlobHook: async (blob) => {
        try {
          setPreviewStatus("загрузка", "loading");
          const data = await uploadOne(blob);
          insertUploadedMedia(data);
          setPreviewStatus("вставлено", "ok");
        } catch (_e) {
          setPreviewStatus("ошибка загрузки", "fallback");
        }
        return false;
      },
    },
  });

  // --- Event bindings --------------------------------------------------------

  toastEditor.on("change", () => {
    syncMarkdown();
    setPreviewStatus("live", "ok");
    checkWikiAutocomplete();
    updateWordCount();
  });

  bindDropAndPaste();

  // Intercept keyboard events in the editor for autocomplete navigation.
  const editorRoot = host.querySelector(".toastui-editor-defaultUI") || host;
  editorRoot.addEventListener("keydown", handleWikiKeydown, true);

  // Dismiss autocomplete on click outside.
  document.addEventListener("click", (e) => {
    if (wikiDropdown && wikiDropdown.style.display !== "none") {
      if (!wikiDropdown.contains(e.target) && !host.contains(e.target)) {
        hideWikiDropdown();
      }
    }
  });

  // --- Initial state ---------------------------------------------------------

  syncMarkdown();
  lastSavedMarkdown = md.value;
  lastAutoSavedMarkdown = md.value;
  setPreviewStatus(tableMergedCell ? "live + tables" : "live", "ok");

  // --- Start word count updates --------------------------------------------
  updateWordCount();
  setInterval(updateWordCount, 3000);
}

// =============================================================================
//  Source mode toggle (Obsidian-style — edit raw Markdown)
// =============================================================================

let sourceModeActive = false;
let sourceTextarea = null;

function toggleSourceMode() {
  if (!host) return;
  const btn = document.getElementById("btnSourceMode");

  if (sourceModeActive) {
    // Switch back to WYSIWYG
    if (sourceTextarea && toastEditor) {
      toastEditor.setMarkdown(sourceTextarea.value, false);
      sourceTextarea.remove();
      sourceTextarea = null;
    }
    host.style.display = "";
    sourceModeActive = false;
    if (btn) {
      btn.textContent = "</>";
      btn.style.background = "";
    }
    setPreviewStatus("live", "ok");
  } else {
    // Switch to source mode
    const markdown = toastEditor ? toastEditor.getMarkdown() : md.value;
    host.style.display = "none";

    sourceTextarea = document.createElement("textarea");
    sourceTextarea.className = "source-editor-textarea";
    sourceTextarea.value = markdown;
    sourceTextarea.spellcheck = false;
    sourceTextarea.addEventListener("input", () => {
      md.value = sourceTextarea.value;
      lastSavedMarkdown = sourceTextarea.value;
      lastAutoSavedMarkdown = sourceTextarea.value;
      updateWordCount();
    });
    host.parentNode.insertBefore(sourceTextarea, host.nextSibling);
    sourceTextarea.focus();
    sourceModeActive = true;
    if (btn) {
      btn.textContent = "👁";
      btn.style.background = "var(--accent-soft)";
    }
    setPreviewStatus("source", "ok");
  }
}

// =============================================================================
//  Focus mode (Obsidian-style — dim non-active blocks)
// =============================================================================

let focusModeActive = false;

function toggleFocusMode() {
  const shell = document.querySelector(".live-editor-shell");
  const btn = document.getElementById("btnFocusMode");
  focusModeActive = !focusModeActive;
  if (focusModeActive) {
    shell.classList.add("editor-focus-mode");
    if (btn) {
      btn.style.background = "var(--accent-soft)";
      btn.textContent = "⊙";
    }
  } else {
    shell.classList.remove("editor-focus-mode");
    if (btn) {
      btn.style.background = "";
      btn.textContent = "⊙";
    }
  }
}

// =============================================================================
//  Word count & reading time (Obsidian-style)
// =============================================================================

function updateWordCount() {
  const wc = document.getElementById("wordCount");
  const rt = document.getElementById("readTime");
  if (!wc && !rt) return;

  let text = "";
  if (sourceModeActive && sourceTextarea) {
    text = sourceTextarea.value;
  } else if (toastEditor) {
    text = toastEditor.getMarkdown();
  } else {
    text = md.value;
  }

  // Count words (Cyrillic + Latin)
  const words = (text || "")
    .trim()
    .replace(/[#*`\[\]()>\-|]/g, " ")
    .split(/\s+/)
    .filter((w) => w.length > 0);
  const wordCount = words.length;

  // Reading time: ~200 words/min average
  const minutes = Math.max(1, Math.ceil(wordCount / 200));

  if (wc) wc.textContent = wordCount + " слов";
  if (rt) rt.textContent = "~" + minutes + " мин";
}

// =============================================================================
//  Boot
// =============================================================================

if (host) {
  initToastEditor();

  // Manual save on form submit.
  editorForm?.addEventListener("submit", () => {
    syncMarkdown();
    lastSavedMarkdown = md.value;
  });

  // Slug auto-fill from title.
  titleInput?.addEventListener("input", autoFillSlug);

  // Ctrl/Cmd+S shortcut.
  document.addEventListener("keydown", (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
      e.preventDefault();
      syncMarkdown();
      lastSavedMarkdown = md.value;
      editorForm?.submit();
    }
  });

  // Start periodic auto-save.
  startAutoSave();
}
