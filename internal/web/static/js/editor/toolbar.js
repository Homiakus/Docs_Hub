/* Editor Toolbar Helper Module */

document.addEventListener('DOMContentLoaded', () => {
  const toolbarButtons = document.querySelectorAll('[data-editor-action]');
  const editorArea = document.getElementById('content');

  if (!editorArea || toolbarButtons.length === 0) return;

  toolbarButtons.forEach((btn) => {
    btn.addEventListener('click', (e) => {
      e.preventDefault();
      const action = btn.getAttribute('data-editor-action');
      applyFormatting(editorArea, action);
    });
  });

  function applyFormatting(textarea, action) {
    const start = textarea.selectionStart ?? 0;
    const end = textarea.selectionEnd ?? 0;
    const selection = textarea.value.substring(start, end);
    let replacement = '';

    const bt = "```";

    switch (action) {
      case 'bold':
        replacement = `**${selection || 'выделенный текст'}**`;
        break;
      case 'italic':
        replacement = `*${selection || 'курсивный текст'}*`;
        break;
      case 'h2':
        replacement = `\n## ${selection || 'Заголовок раздела'}\n`;
        break;
      case 'h3':
        replacement = `\n### ${selection || 'Подзаголовок'}\n`;
        break;
      case 'list':
        replacement = `\n- ${selection || 'Элемент списка'}\n`;
        break;
      case 'code':
        replacement = `\`${selection || 'код'}\``;
        break;
      case 'codeblock':
        replacement = `\n${bt}go\n${selection || '// Исходный код'}\n${bt}\n`;
        break;
      case 'quote':
        replacement = `\n> ${selection || 'Цитата или примечание'}\n`;
        break;
      case 'table':
        replacement = `\n| Колонка 1 | Колонка 2 |\n|---|---|\n| Ячейка 1 | Ячейка 2 |\n`;
        break;
      case 'mermaid':
        replacement = `\n${bt}mermaid\ngraph TD\n    A[Начало] --> B[Процесс]\n${bt}\n`;
        break;
      default:
        return;
    }

    textarea.value = textarea.value.substring(0, start) + replacement + textarea.value.substring(end);
    textarea.focus();
    const newCursor = start + replacement.length;
    textarea.setSelectionRange(newCursor, newCursor);
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
  }
});
