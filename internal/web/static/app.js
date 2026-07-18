(() => {
  const navToggle = document.querySelector('.mobile-nav-toggle');
  const navClose = document.querySelector('.mobile-nav-close');
  const sidepanel = document.querySelector('.sidepanel');
  const backdrop = document.querySelector('.mobile-backdrop');
  const mobileNavigation = window.matchMedia('(max-width: 760px)');
  let navPreviousFocus = null;

  function setNavOpen(open) {
    document.body.classList.toggle('nav-open', open);
    navToggle?.setAttribute('aria-expanded', String(open));
    if (sidepanel) sidepanel.inert = mobileNavigation.matches && !open;
    if (open) {
      navPreviousFocus = document.activeElement;
      navClose?.focus();
    } else if (document.activeElement === navClose) {
      navPreviousFocus?.focus();
    }
  }
  const syncNavigationMode = () => {
    if (!mobileNavigation.matches) {
      document.body.classList.remove('nav-open');
      navToggle?.setAttribute('aria-expanded', 'false');
      if (sidepanel) sidepanel.inert = false;
    } else if (!document.body.classList.contains('nav-open') && sidepanel) {
      sidepanel.inert = true;
    }
  };
  syncNavigationMode();
  mobileNavigation.addEventListener?.('change', syncNavigationMode);
  navToggle?.addEventListener('click', () => setNavOpen(!document.body.classList.contains('nav-open')));
  navClose?.addEventListener('click', () => setNavOpen(false));
  backdrop?.addEventListener('click', () => setNavOpen(false));
  sidepanel?.addEventListener('click', (event) => {
    if (event.target.closest('a,button[type="submit"]')) setNavOpen(false);
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && document.body.classList.contains('nav-open')) setNavOpen(false);
  });

  const themeToggle = document.getElementById('themeToggle');
  updateThemeIcon(document.documentElement.dataset.theme || 'light');
  themeToggle?.addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem('docs-hub-theme', next); } catch (_) {}
    updateThemeIcon(next);
  });
  function updateThemeIcon(theme) {
    const icon = themeToggle?.querySelector('.theme-icon');
    if (icon) icon.textContent = theme === 'light' ? '☾' : '☀';
  }

  const csrfToken = () => document.querySelector('meta[name="csrf-token"]')?.content || '';
  const apiFetch = (url, options = {}) => {
    const headers = new Headers(options.headers || {});
    if (csrfToken() && !headers.has('X-CSRF-Token')) headers.set('X-CSRF-Token', csrfToken());
    return fetch(url, { ...options, headers });
  };

  function enhanceMarkdown(root = document) {
    root.querySelectorAll('.markdown table').forEach((table) => {
      if (table.closest('.table-wrap')) return;
      const wrapper = document.createElement('div');
      wrapper.className = 'table-wrap';
      table.before(wrapper);
      wrapper.appendChild(table);
    });
    root.querySelectorAll('.markdown pre').forEach((pre) => {
      if (pre.querySelector('.code-copy')) return;
      const button = document.createElement('button');
      button.className = 'code-copy';
      button.type = 'button';
      button.textContent = 'Копировать';
      button.setAttribute('aria-label', 'Копировать блок кода');
      button.addEventListener('click', async () => {
        const text = pre.querySelector('code')?.textContent || '';
        try {
          await navigator.clipboard.writeText(text);
          button.textContent = 'Скопировано';
          window.setTimeout(() => { button.textContent = 'Копировать'; }, 1800);
        } catch (_) {
          showToast('Не удалось скопировать код');
        }
      });
      pre.appendChild(button);
    });
  }
  enhanceMarkdown();

  const editor = document.getElementById('content');
  const preview = document.getElementById('preview');
  if (editor && preview) {
    const dropzone = document.getElementById('dropzone');
    const fileInput = document.getElementById('mediaInput');
    const picker = document.getElementById('mediaPicker');
    let previewController = null;
    const render = async () => {
      previewController?.abort();
      previewController = new AbortController();
      try {
        const response = await apiFetch('/api/preview', {
          method: 'POST', headers: { 'Content-Type': 'text/plain; charset=utf-8' },
          body: editor.value, signal: previewController.signal,
        });
        if (response.redirected && new URL(response.url).pathname === '/login') { window.location.assign('/login'); return; }
        if (!response.ok) throw new Error((await response.text()).trim() || 'Предпросмотр недоступен');
        preview.innerHTML = await response.text();
        enhanceMarkdown(preview);
        if (window.mermaid) {
          const diagrams = preview.querySelectorAll('.mermaid:not([data-processed])');
          if (diagrams.length) await window.mermaid.run({ nodes: diagrams });
        }
      } catch (error) {
        if (error.name !== 'AbortError') showToast(error.message || 'Не удалось обновить предпросмотр');
      }
    };
    const scheduleRender = debounce(render, 220);
    editor.addEventListener('input', scheduleRender);
    picker?.addEventListener('click', () => fileInput?.click());
    fileInput?.addEventListener('change', () => {
      uploadFiles(Array.from(fileInput.files || []), editor, render, dropzone).finally(() => { fileInput.value = ''; });
    });
    [dropzone, editor].filter(Boolean).forEach((target) => {
      target.addEventListener('dragenter', (event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        dropzone?.classList.add('is-dragging');
      });
      target.addEventListener('dragover', (event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = 'copy';
      });
      target.addEventListener('dragleave', (event) => {
        if (!dropzone?.contains(event.relatedTarget)) dropzone?.classList.remove('is-dragging');
      });
      target.addEventListener('drop', (event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        dropzone?.classList.remove('is-dragging');
        uploadFiles(Array.from(event.dataTransfer.files || []), editor, render, dropzone);
      });
    });
    render();
  }

  document.addEventListener('keydown', (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
      const form = document.activeElement?.closest?.('form.editor') || document.querySelector('form.editor');
      if (form) { event.preventDefault(); form.requestSubmit(); }
    }
    if (event.key === '/' && !event.ctrlKey && !event.metaKey && !event.altKey) {
      const tag = document.activeElement?.tagName;
      if (!['INPUT', 'TEXTAREA', 'SELECT'].includes(tag)) {
        event.preventDefault();
        document.getElementById('globalSearch')?.focus();
      }
    }
  });

  async function uploadFiles(files, targetEditor, render, dropzone) {
    const allowed = files.filter(isSupportedFile);
    if (!allowed.length) { showToast('Этот формат файла не поддерживается'); return; }
    dropzone?.classList.add('is-uploading');
    try {
      for (const file of allowed) {
        const form = new FormData();
        form.append('file', file);
        const response = await apiFetch(targetEditor.dataset.uploadEndpoint || '/api/uploads', { method: 'POST', body: form });
        if (!response.ok) throw new Error((await response.text()).trim() || `Не удалось загрузить ${file.name}`);
        const payload = await response.json();
        insertAtCursor(targetEditor, `\n\n${payload.markdown}\n\n`);
      }
      targetEditor.dispatchEvent(new Event('input', { bubbles: true }));
      await render();
      targetEditor.focus();
      showToast(allowed.length === 1 ? 'Файл добавлен' : `Добавлено файлов: ${allowed.length}`);
    } catch (error) {
      showToast(error.message || 'Не удалось загрузить вложение');
    } finally {
      dropzone?.classList.remove('is-uploading');
    }
  }

  function insertAtCursor(textarea, text) {
    const start = textarea.selectionStart ?? textarea.value.length;
    const end = textarea.selectionEnd ?? start;
    textarea.setRangeText(text, start, end, 'end');
  }
  function isSupportedFile(file) {
    if (/^(image|audio|video)\//.test(file.type || '') || file.type === 'application/pdf') return true;
    return /\.(avif|gif|jpe?g|png|webp|aac|flac|m4a|mp3|oga|ogg|wav|webm|mov|m4v|mp4|pdf)$/i.test(file.name || '');
  }
  function hasFiles(event) { return Array.from(event.dataTransfer?.types || []).includes('Files'); }
  function debounce(fn, delay) { let timer; return (...args) => { window.clearTimeout(timer); timer = window.setTimeout(() => fn(...args), delay); }; }
  function showToast(message) {
    let region = document.querySelector('.toast-region');
    if (!region) {
      region = document.createElement('div');
      region.className = 'toast-region';
      region.setAttribute('aria-live', 'polite');
      document.body.appendChild(region);
    }
    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.setAttribute('role', 'status');
    toast.textContent = message;
    region.appendChild(toast);
    window.setTimeout(() => {
      toast.classList.add('is-hiding');
      window.setTimeout(() => toast.remove(), 260);
    }, 3300);
  }
})();
