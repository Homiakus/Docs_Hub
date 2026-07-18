document.addEventListener('DOMContentLoaded', () => {
  const overlay = document.createElement('div');
  overlay.id = 'commandPaletteOverlay';
  overlay.className = 'dialog-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = `
    <section class="dialog-content command-dialog" role="dialog" aria-modal="true" aria-labelledby="commandPaletteTitle">
      <h2 class="sr-only" id="commandPaletteTitle">Быстрый поиск и команды</h2>
      <div class="command-search"><span aria-hidden="true">⌕</span><input id="commandPaletteInput" type="search" role="combobox" aria-autocomplete="list" aria-expanded="false" aria-controls="commandPaletteResults" placeholder="Найти документ или выполнить команду"><kbd>Esc</kbd></div>
      <div class="command-results" id="commandPaletteResults" role="listbox" aria-label="Результаты"></div>
      <footer class="command-footer"><span><kbd>↑</kbd><kbd>↓</kbd> выбрать</span><span><kbd>Enter</kbd> открыть</span><span><kbd>Esc</kbd> закрыть</span></footer>
    </section>`;
  document.body.appendChild(overlay);

  const dialog = overlay.querySelector('[role="dialog"]');
  const input = overlay.querySelector('#commandPaletteInput');
  const results = overlay.querySelector('#commandPaletteResults');
  const trigger = document.getElementById('commandPaletteButton');
  let options = [];
  let activeIndex = -1;
  let previousFocus = null;
  let timer = 0;
  let controller = null;

  const quickActions = [
    ...(document.querySelector('a[href="/new"]') ? [{ title: 'Создать документ', meta: 'Новый черновик', href: '/new', icon: '＋' }] : []),
    { title: 'Расширенный поиск', meta: 'Фильтры и статусы', href: '/search', icon: '⌕' },
    { title: 'Пространства', meta: 'Структура базы знаний', href: '/spaces', icon: '▦' },
    { title: 'Граф знаний', meta: 'Связи между документами', href: '/graph', icon: '⌘' },
  ];

  function open() {
    previousFocus = document.activeElement;
    overlay.classList.add('open');
    overlay.setAttribute('aria-hidden', 'false');
    input.setAttribute('aria-expanded', 'true');
    document.body.style.overflow = 'hidden';
    input.value = '';
    render(quickActions, 'Быстрые действия');
    window.setTimeout(() => input.focus(), 0);
  }

  function close() {
    controller?.abort();
    window.clearTimeout(timer);
    overlay.classList.remove('open');
    overlay.setAttribute('aria-hidden', 'true');
    input.setAttribute('aria-expanded', 'false');
    input.removeAttribute('aria-activedescendant');
    document.body.style.overflow = '';
    previousFocus?.focus();
  }

  function render(items, label) {
    options = [];
    activeIndex = -1;
    results.replaceChildren();
    const groupLabel = document.createElement('div');
    groupLabel.className = 'command-group-label';
    groupLabel.textContent = label;
    results.appendChild(groupLabel);
    if (!items.length) {
      const empty = document.createElement('div');
      empty.className = 'command-empty';
      empty.textContent = 'Ничего не найдено. Попробуйте изменить запрос.';
      results.appendChild(empty);
      return;
    }
    items.forEach((item, index) => {
      const option = document.createElement('a');
      option.id = `command-option-${index}`;
      option.className = 'command-option';
      option.href = item.href || `/a/${encodeURIComponent(item.slug)}`;
      option.setAttribute('role', 'option');
      option.setAttribute('aria-selected', 'false');
      const icon = document.createElement('span');
      icon.setAttribute('aria-hidden', 'true');
      icon.textContent = item.icon || '≡';
      const copy = document.createElement('div');
      const title = document.createElement('strong');
      title.textContent = item.title;
      const meta = document.createElement('small');
      meta.textContent = item.meta || item.slug || '';
      copy.append(title, meta);
      const arrow = document.createElement('span');
      arrow.setAttribute('aria-hidden', 'true');
      arrow.textContent = '↗';
      option.append(icon, copy, arrow);
      option.addEventListener('mousemove', () => setActive(index));
      results.appendChild(option);
      options.push(option);
    });
  }

  function setActive(index) {
    if (!options.length) return;
    activeIndex = (index + options.length) % options.length;
    options.forEach((option, optionIndex) => {
      const active = optionIndex === activeIndex;
      option.classList.toggle('active', active);
      option.setAttribute('aria-selected', String(active));
    });
    const active = options[activeIndex];
    input.setAttribute('aria-activedescendant', active.id);
    active.scrollIntoView({ block: 'nearest' });
  }

  async function search(query) {
    controller?.abort();
    controller = new AbortController();
    try {
      const response = await fetch(`/api/v1/search/suggest?q=${encodeURIComponent(query)}`, { signal: controller.signal });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      render((data.suggestions || []).map((item) => ({ ...item, meta: `/a/${item.slug}`, icon: '≡' })), 'Документы');
    } catch (error) {
      if (error.name === 'AbortError') return;
      render([], 'Ошибка поиска');
    }
  }

  input.addEventListener('input', () => {
    window.clearTimeout(timer);
    const query = input.value.trim();
    if (!query) { render(quickActions, 'Быстрые действия'); return; }
    timer = window.setTimeout(() => search(query), 140);
  });

  input.addEventListener('keydown', (event) => {
    if (event.key === 'ArrowDown') { event.preventDefault(); setActive(activeIndex + 1); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); setActive(activeIndex < 0 ? options.length - 1 : activeIndex - 1); }
    else if (event.key === 'Enter' && activeIndex >= 0) { event.preventDefault(); options[activeIndex].click(); }
  });

  document.addEventListener('keydown', (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      overlay.classList.contains('open') ? close() : open();
      return;
    }
    if (event.key === 'Escape' && overlay.classList.contains('open')) { event.preventDefault(); close(); }
  });
  trigger?.addEventListener('click', open);
  overlay.addEventListener('click', (event) => { if (event.target === overlay) close(); });
  dialog.addEventListener('keydown', (event) => {
    if (event.key !== 'Tab') return;
    const focusable = [input, ...options];
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  });
});
