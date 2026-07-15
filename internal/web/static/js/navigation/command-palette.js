/* Command Palette (Ctrl+K) Navigation Module */

document.addEventListener('DOMContentLoaded', () => {
  // Create Command Palette Modal DOM
  const paletteHTML = `
    <div id="commandPaletteOverlay" class="dialog-overlay" aria-hidden="true">
      <div class="dialog-content" style="max-width: 600px; padding: 0; overflow: hidden;">
        <div style="padding: var(--space-3) var(--space-4); border-bottom: 1px solid var(--border-subtle); display: flex; align-items: center; gap: var(--space-2);">
          <span style="color: var(--text-tertiary);">🔍</span>
          <input type="text" id="commandPaletteInput" placeholder="Поиск по документам или введите команду... (Esc для выхода)" 
                 style="border: none; background: transparent; font-size: 1rem; padding: var(--space-2) 0; box-shadow: none;" autofocus>
        </div>
        <div id="commandPaletteResults" style="max-height: 360px; overflow-y: auto; padding: var(--space-2);">
          <div style="padding: var(--space-3); font-size: 0.875rem; color: var(--text-tertiary);">Начните вводить текст для поиска...</div>
        </div>
      </div>
    </div>
  `;

  document.body.insertAdjacentHTML('beforeend', paletteHTML);

  const overlay = document.getElementById('commandPaletteOverlay');
  const input = document.getElementById('commandPaletteInput');
  const resultsContainer = document.getElementById('commandPaletteResults');

  function openPalette() {
    overlay.classList.add('open');
    overlay.setAttribute('aria-hidden', 'false');
    input.focus();
    input.select();
  }

  function closePalette() {
    overlay.classList.remove('open');
    overlay.setAttribute('aria-hidden', 'true');
  }

  // Keyboard shortcut binding: Ctrl+K or Cmd+K
  document.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      if (overlay.classList.contains('open')) {
        closePalette();
      } else {
        openPalette();
      }
    } else if (e.key === 'Escape' && overlay.classList.contains('open')) {
      closePalette();
    }
  });

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) closePalette();
  });

  // Debounced search suggest API integration
  let debounceTimer;
  input.addEventListener('input', () => {
    clearTimeout(debounceTimer);
    const query = input.value.trim();
    if (!query) {
      resultsContainer.innerHTML = '<div style="padding: var(--space-3); font-size: 0.875rem; color: var(--text-tertiary);">Начните вводить текст для поиска...</div>';
      return;
    }

    debounceTimer = setTimeout(async () => {
      try {
        const resp = await fetch(`/api/v1/search/suggest?q=${encodeURIComponent(query)}`);
        if (!resp.ok) return;
        const data = await resp.json();
        renderResults(data.suggestions || []);
      } catch (err) {
        console.error('Command palette suggest error:', err);
      }
    }, 150);
  });

  function renderResults(items) {
    if (items.length === 0) {
      resultsContainer.innerHTML = '<div style="padding: var(--space-3); font-size: 0.875rem; color: var(--text-tertiary);">Ничего не найдено</div>';
      return;
    }
    resultsContainer.innerHTML = items.map(item => `
      <a href="/a/${encodeURIComponent(item.slug)}" class="side-link" style="padding: var(--space-3); display: block; text-decoration: none;">
        <span style="font-weight: 500; color: var(--text-primary);">${escapeHTML(item.title)}</span>
        <span style="font-size: 0.75rem; color: var(--text-tertiary); margin-left: var(--space-2);">/a/${escapeHTML(item.slug)}</span>
      </a>
    `).join('');
  }

  function escapeHTML(str) {
    return String(str).replace(/[&<>"']/g, m => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[m]);
  }
});
