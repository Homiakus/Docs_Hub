document.addEventListener('DOMContentLoaded', () => {
  const editor = document.getElementById('content');
  const trigger = document.querySelector('[data-editor-action="table"]');
  if (!editor || !trigger) return;

  const overlay = document.createElement('div');
  overlay.id = 'tableBuilderOverlay';
  overlay.className = 'dialog-overlay';
  overlay.setAttribute('aria-hidden', 'true');
  overlay.innerHTML = `
    <section class="dialog-content" role="dialog" aria-modal="true" aria-labelledby="tableBuilderTitle" aria-describedby="tableBuilderDescription">
      <header class="dialog-header"><div><h2 class="dialog-title" id="tableBuilderTitle">Конструктор таблицы</h2><p class="sr-only" id="tableBuilderDescription">Настройте размер, выравнивание и содержимое будущей Markdown-таблицы.</p></div><button class="icon-button" type="button" data-close aria-label="Закрыть">×</button></header>
      <div class="dialog-body">
        <div class="table-builder-controls">
          <label>Строки<input class="form-control" type="number" id="tblRows" value="3" min="1" max="12"></label>
          <label>Колонки<input class="form-control" type="number" id="tblCols" value="3" min="1" max="8"></label>
          <label>Выравнивание<select class="form-control" id="tblAlign"><option value="left">По левому краю</option><option value="center">По центру</option><option value="right">По правому краю</option></select></label>
        </div>
        <div class="table-preview-wrap"><div class="table-preview" id="tablePreview" aria-label="Содержимое таблицы"></div></div>
      </div>
      <footer class="dialog-footer"><button class="btn btn-secondary" type="button" data-cancel>Отмена</button><button class="btn btn-primary" type="button" data-insert>Вставить таблицу</button></footer>
    </section>`;
  document.body.appendChild(overlay);

  const dialog = overlay.querySelector('[role="dialog"]');
  const rowsInput = overlay.querySelector('#tblRows');
  const colsInput = overlay.querySelector('#tblCols');
  const alignInput = overlay.querySelector('#tblAlign');
  const preview = overlay.querySelector('#tablePreview');
  let previousFocus = null;

  const clamp = (value, min, max) => Math.min(max, Math.max(min, Number(value) || min));
  const currentValues = () => {
    const values = new Map();
    preview.querySelectorAll('input[data-cell]').forEach((input) => values.set(input.dataset.cell, input.value));
    return values;
  };

  function renderPreview() {
    const saved = currentValues();
    const rows = clamp(rowsInput.value, 1, 12);
    const cols = clamp(colsInput.value, 1, 8);
    rowsInput.value = rows;
    colsInput.value = cols;
    preview.style.gridTemplateColumns = `repeat(${cols}, minmax(110px, 1fr))`;
    preview.replaceChildren();
    for (let row = 0; row <= rows; row += 1) {
      for (let col = 0; col < cols; col += 1) {
        const key = `${row}:${col}`;
        const input = document.createElement('input');
        input.type = 'text';
        input.dataset.cell = key;
        input.className = row === 0 ? 'is-header' : '';
        input.setAttribute('aria-label', row === 0 ? `Заголовок колонки ${col + 1}` : `Строка ${row}, колонка ${col + 1}`);
        input.value = saved.get(key) ?? (row === 0 ? `Колонка ${col + 1}` : '');
        preview.appendChild(input);
      }
    }
  }

  function open() {
    previousFocus = document.activeElement;
    overlay.classList.add('open');
    overlay.setAttribute('aria-hidden', 'false');
    document.body.style.overflow = 'hidden';
    renderPreview();
    rowsInput.focus();
    rowsInput.select();
  }

  function close() {
    overlay.classList.remove('open');
    overlay.setAttribute('aria-hidden', 'true');
    document.body.style.overflow = '';
    previousFocus?.focus();
  }

  function markdown() {
    const rows = clamp(rowsInput.value, 1, 12);
    const cols = clamp(colsInput.value, 1, 8);
    const align = alignInput.value;
    const cell = (row, col) => {
      const value = preview.querySelector(`[data-cell="${row}:${col}"]`)?.value.trim() || (row === 0 ? `Колонка ${col + 1}` : '');
      return value.replace(/\|/g, '\\|').replace(/\n/g, ' ');
    };
    const lines = [];
    lines.push(`| ${Array.from({ length: cols }, (_, col) => cell(0, col)).join(' | ')} |`);
    const marker = align === 'center' ? ':---:' : align === 'right' ? '---:' : '---';
    lines.push(`| ${Array.from({ length: cols }, () => marker).join(' | ')} |`);
    for (let row = 1; row <= rows; row += 1) {
      lines.push(`| ${Array.from({ length: cols }, (_, col) => cell(row, col)).join(' | ')} |`);
    }
    return lines.join('\n');
  }

  trigger.addEventListener('click', (event) => { event.preventDefault(); open(); });
  rowsInput.addEventListener('input', renderPreview);
  colsInput.addEventListener('input', renderPreview);
  overlay.querySelector('[data-close]').addEventListener('click', close);
  overlay.querySelector('[data-cancel]').addEventListener('click', close);
  overlay.addEventListener('click', (event) => { if (event.target === overlay) close(); });
  overlay.querySelector('[data-insert]').addEventListener('click', () => {
    const value = `\n\n${markdown()}\n\n`;
    const start = editor.selectionStart ?? editor.value.length;
    const end = editor.selectionEnd ?? start;
    editor.setRangeText(value, start, end, 'end');
    editor.dispatchEvent(new Event('input', { bubbles: true }));
    close();
    editor.focus();
  });

  overlay.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') { event.preventDefault(); close(); return; }
    if (event.key !== 'Tab') return;
    const focusable = Array.from(dialog.querySelectorAll('button:not(:disabled),input:not(:disabled),select:not(:disabled)'));
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  });
});
