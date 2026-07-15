/* Visual Table Builder Module */

document.addEventListener('DOMContentLoaded', () => {
  const dialogHTML = `
    <div id="tableBuilderOverlay" class="dialog-overlay" aria-hidden="true">
      <div class="dialog-content">
        <div class="dialog-header">
          <h3 class="dialog-title">Конструктор таблиц</h3>
          <button class="icon-button" id="closeTableBuilder" type="button" aria-label="Закрыть">×</button>
        </div>
        <div class="dialog-body" style="display: grid; gap: var(--space-4);">
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-3);">
            <label class="form-label">Строки
              <input type="number" id="tblRows" value="3" min="1" max="50" class="form-control">
            </label>
            <label class="form-label">Колонки
              <input type="number" id="tblCols" value="3" min="1" max="20" class="form-control">
            </label>
          </div>
          <div>
            <label class="form-label">Выравнивание текста
              <select id="tblAlign" class="form-control">
                <option value="left">По левому краю (left)</option>
                <option value="center">По центру (center)</option>
                <option value="right">По правому краю (right)</option>
              </select>
            </label>
          </div>
        </div>
        <div class="dialog-footer">
          <button type="button" class="btn btn-secondary" id="cancelTableBuilder">Отмена</button>
          <button type="button" class="btn btn-primary" id="insertTableBtn">Вставить таблицу</button>
        </div>
      </div>
    </div>
  `;

  document.body.insertAdjacentHTML('beforeend', dialogHTML);

  const overlay = document.getElementById('tableBuilderOverlay');
  const closeBtn = document.getElementById('closeTableBuilder');
  const cancelBtn = document.getElementById('cancelTableBuilder');
  const insertBtn = document.getElementById('insertTableBtn');
  const editorArea = document.getElementById('content');

  // Trigger from toolbar
  const tableTriggerBtn = document.querySelector('[data-editor-action="table"]');
  if (tableTriggerBtn) {
    tableTriggerBtn.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      openModal();
    });
  }

  function openModal() {
    overlay.classList.add('open');
    overlay.setAttribute('aria-hidden', 'false');
  }

  function closeModal() {
    overlay.classList.remove('open');
    overlay.setAttribute('aria-hidden', 'true');
  }

  closeBtn?.addEventListener('click', closeModal);
  cancelBtn?.addEventListener('click', closeModal);

  insertBtn?.addEventListener('click', () => {
    const rows = parseInt(document.getElementById('tblRows').value || '3', 10);
    const cols = parseInt(document.getElementById('tblCols').value || '3', 10);
    const align = document.getElementById('tblAlign').value;

    const markdownTable = generateMarkdownTable(rows, cols, align);
    if (editorArea) {
      const start = editorArea.selectionStart ?? editorArea.value.length;
      editorArea.value = editorArea.value.substring(0, start) + `\n${markdownTable}\n` + editorArea.value.substring(start);
      editorArea.dispatchEvent(new Event('input', { bubbles: true }));
    }

    closeModal();
  });

  function generateMarkdownTable(r, c, align) {
    let headerRow = '|';
    let alignRow = '|';
    for (let j = 1; j <= c; j++) {
      headerRow += ` Заголовок ${j} |`;
      if (align === 'center') alignRow += ' :---: |';
      else if (align === 'right') alignRow += ' ---: |';
      else alignRow += ' --- |';
    }

    let bodyRows = '';
    for (let i = 1; i <= r; i++) {
      let row = '|';
      for (let j = 1; j <= c; j++) {
        row += ` Данные ${i}-${j} |`;
      }
      bodyRows += `${row}\n`;
    }

    return `${headerRow}\n${alignRow}\n${bodyRows}`;
  }
});
