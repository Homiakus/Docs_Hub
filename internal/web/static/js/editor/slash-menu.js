/**
 * Docs_Hub — Editor Slash Commands Menu & Inline Component Palette
 */
class EditorSlashMenu {
  constructor(editorEl) {
    this.editor = editorEl;
    if (!this.editor) return;

    this.commands = [
      { id: 'h2', label: 'Заголовок H2', desc: 'Основной раздел документа', icon: 'H2', template: '\n## Заголовок раздела\n' },
      { id: 'h3', label: 'Подзаголовок H3', desc: 'Вложенный подраздел', icon: 'H3', template: '\n### Подзаголовок\n' },
      { id: 'callout-note', label: 'Примечание (Note)', desc: 'Информационная плашка > [!NOTE]', icon: 'ℹ️', template: '\n> [!NOTE]\n> Важная контекстная информация.\n' },
      { id: 'callout-warning', label: 'Предупреждение (Warning)', desc: 'Предупреждающая плашка > [!WARNING]', icon: '⚠️', template: '\n> [!WARNING]\n> Критически важная информация перед действием.\n' },
      { id: 'callout-tip', label: 'Совет (Tip)', desc: 'Полезная подсказка > [!TIP]', icon: '💡', template: '\n> [!TIP]\n> Оптимизация или совет по настройке.\n' },
      { id: 'table', label: 'Таблица Markdown', desc: 'Таблица с заголовками и строками', icon: '▦', template: '\n| Колонка 1 | Колонка 2 | Колонка 3 |\n| :--- | :---: | ---: |\n| Значение A | Значение B | Значение C |\n' },
      { id: 'mermaid', label: 'Диаграмма Mermaid', desc: 'Блок схемы процессов graph TD', icon: '☷', template: '\n```mermaid\ngraph TD\n    A[Начало] --> B[Процесс]\n    B --> C[Результат]\n```\n' },
      { id: 'codeblock', label: 'Блок кода', desc: 'Синтаксис кода ```go', icon: '{ }', template: '\n```go\n// Код функции\n```\n' },
      { id: 'task', label: 'Список задач', desc: 'Интерактивные чекбоксы', icon: '☑', template: '\n- [ ] Первая задача\n- [ ] Вторая задача\n' },
      { id: 'quote', label: 'Цитата', desc: 'Блок выноски цитаты', icon: '❞', template: '\n> Цитата или ключевая мысль автора.\n' },
      { id: 'wikilink', label: 'Wiki-ссылка [[slug]]', desc: 'Внутренняя ссылка на статью', icon: '🔗', template: '[[slug|Название статьи]]' }
    ];

    this.selectedIndex = 0;
    this.menuEl = null;
    this.query = '';
    this.active = false;
    this.init();
  }

  init() {
    this.createMenuDOM();
    this.bindEvents();
  }

  createMenuDOM() {
    this.menuEl = document.createElement('div');
    this.menuEl.className = 'slash-command-menu';
    this.menuEl.setAttribute('role', 'listbox');
    this.menuEl.style.display = 'none';
    document.body.appendChild(this.menuEl);
  }

  bindEvents() {
    this.editor.addEventListener('input', (e) => this.handleInput(e));
    this.editor.addEventListener('keydown', (e) => this.handleKeydown(e));
    document.addEventListener('click', (e) => {
      if (this.active && !this.menuEl.contains(e.target) && e.target !== this.editor) {
        this.close();
      }
    });
  }

  handleInput() {
    const pos = this.editor.selectionStart;
    const val = this.editor.value;
    const lineStart = val.lastIndexOf('\n', pos - 1) + 1;
    const currentLine = val.substring(lineStart, pos);

    const slashMatch = currentLine.match(/^\/([a-zA-Zа-яА-Я0-9-_]*)$/);
    if (slashMatch) {
      this.query = slashMatch[1].toLowerCase();
      this.open();
      this.render();
    } else if (this.active) {
      this.close();
    }
  }

  handleKeydown(e) {
    if (!this.active) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      const filtered = this.getFiltered();
      if (filtered.length > 0) {
        this.selectedIndex = (this.selectedIndex + 1) % filtered.length;
        this.render();
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      const filtered = this.getFiltered();
      if (filtered.length > 0) {
        this.selectedIndex = (this.selectedIndex - 1 + filtered.length) % filtered.length;
        this.render();
      }
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      const filtered = this.getFiltered();
      if (filtered.length > 0) {
        e.preventDefault();
        this.executeCommand(filtered[this.selectedIndex]);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      this.close();
    }
  }

  getFiltered() {
    if (!this.query) return this.commands;
    return this.commands.filter(c => 
      c.id.includes(this.query) || 
      c.label.toLowerCase().includes(this.query) || 
      c.desc.toLowerCase().includes(this.query)
    );
  }

  open() {
    this.active = true;
    this.selectedIndex = 0;
    this.positionMenu();
    this.menuEl.style.display = 'block';
  }

  close() {
    this.active = false;
    if (this.menuEl) this.menuEl.style.display = 'none';
  }

  positionMenu() {
    const rect = this.editor.getBoundingClientRect();
    this.menuEl.style.left = `${window.scrollX + rect.left + 24}px`;
    this.menuEl.style.top = `${window.scrollY + rect.top + 60}px`;
  }

  render() {
    const items = this.getFiltered();
    if (items.length === 0) {
      this.menuEl.innerHTML = `<div class="slash-empty">Команда не найдена</div>`;
      return;
    }

    this.menuEl.innerHTML = items.map((c, i) => `
      <div class="slash-item ${i === this.selectedIndex ? 'selected' : ''}" data-cmd-id="${c.id}" role="option">
        <span class="slash-icon">${c.icon}</span>
        <div class="slash-details">
          <strong>${c.label}</strong>
          <small>${c.desc}</small>
        </div>
      </div>
    `).join('');

    this.menuEl.querySelectorAll('.slash-item').forEach((itemEl, idx) => {
      itemEl.addEventListener('click', () => {
        this.executeCommand(items[idx]);
      });
      itemEl.addEventListener('mouseenter', () => {
        this.selectedIndex = idx;
        this.render();
      });
    });
  }

  executeCommand(cmd) {
    if (!cmd) return;
    const pos = this.editor.selectionStart;
    const val = this.editor.value;
    const lineStart = val.lastIndexOf('\n', pos - 1) + 1;

    // Replace the slash query on current line with command template
    const before = val.substring(0, lineStart);
    const after = val.substring(pos);

    this.editor.value = before + cmd.template + after;
    this.editor.focus();
    const newPos = before.length + cmd.template.length;
    this.editor.setSelectionRange(newPos, newPos);
    this.editor.dispatchEvent(new Event('input', { bubbles: true }));
    this.close();
  }
}

window.EditorSlashMenu = EditorSlashMenu;

document.addEventListener('DOMContentLoaded', () => {
  const editorEl = document.getElementById('content');
  if (editorEl) {
    new EditorSlashMenu(editorEl);
  }
});
