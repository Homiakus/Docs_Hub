/**
 * Docs_Hub — Editor In-Canvas Drag & Drop / Clipboard Paste Media Upload
 */
class EditorMediaUpload {
  constructor(editorEl, uploadEndpoint = '/api/uploads') {
    this.editor = editorEl;
    this.uploadEndpoint = uploadEndpoint;
    this.csrfToken = document.querySelector('meta[name="csrf-token"]')?.getAttribute('content') || '';
    if (!this.editor) return;
    this.init();
  }

  init() {
    this.bindPaste();
    this.bindDragDrop();
  }

  bindPaste() {
    this.editor.addEventListener('paste', async (e) => {
      const clipboardData = e.clipboardData || window.clipboardData;
      if (!clipboardData || !clipboardData.items) return;

      const files = [];
      for (let i = 0; i < clipboardData.items.length; i++) {
        const item = clipboardData.items[i];
        if (item.kind === 'file') {
          const file = item.getAsFile();
          if (file) files.push(file);
        }
      }

      if (files.length > 0) {
        e.preventDefault();
        for (const f of files) {
          await this.uploadAndInsert(f);
        }
      }
    });
  }

  bindDragDrop() {
    this.editor.addEventListener('dragover', (e) => {
      e.preventDefault();
      this.editor.classList.add('editor-dragover');
    });

    this.editor.addEventListener('dragleave', () => {
      this.editor.classList.remove('editor-dragover');
    });

    this.editor.addEventListener('drop', async (e) => {
      e.preventDefault();
      this.editor.classList.remove('editor-dragover');
      const files = Array.from(e.dataTransfer?.files || []);
      if (files.length > 0) {
        for (const f of files) {
          await this.uploadAndInsert(f);
        }
      }
    });
  }

  async uploadAndInsert(file) {
    const placeholder = `\n![Загрузка ${file.name}...] (Загрузка файла...)\n`;
    const start = this.editor.selectionStart;
    const end = this.editor.selectionEnd;
    const val = this.editor.value;

    // Insert loading placeholder
    this.editor.value = val.substring(0, start) + placeholder + val.substring(end);
    this.editor.dispatchEvent(new Event('input', { bubbles: true }));

    const formData = new FormData();
    formData.append('file', file);
    if (this.csrfToken) {
      formData.append('csrf_token', this.csrfToken);
    }

    try {
      const res = await fetch(this.uploadEndpoint, {
        method: 'POST',
        headers: {
          'X-CSRF-Token': this.csrfToken,
          'Accept': 'application/json'
        },
        body: formData
      });

      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      
      let markdownSnippet = '';
      if (data.markdown) {
        markdownSnippet = data.markdown;
      } else if (file.type.startsWith('image/')) {
        markdownSnippet = `![${file.name}](${data.url || data.path})`;
      } else {
        markdownSnippet = `[${file.name}](${data.url || data.path})`;
      }

      // Replace placeholder with final snippet
      this.editor.value = this.editor.value.replace(placeholder, `\n${markdownSnippet}\n`);
      this.editor.dispatchEvent(new Event('input', { bubbles: true }));

      if (window.ToastManager) {
        window.ToastManager.show('success', `Файл "${file.name}" загружен`);
      }
    } catch (err) {
      this.editor.value = this.editor.value.replace(placeholder, `\n<!-- Ошибка загрузки ${file.name} -->\n`);
      this.editor.dispatchEvent(new Event('input', { bubbles: true }));
      if (window.ToastManager) {
        window.ToastManager.show('error', `Ошибка загрузки ${file.name}`);
      }
    }
  }
}

window.EditorMediaUpload = EditorMediaUpload;

document.addEventListener('DOMContentLoaded', () => {
  const editorEl = document.getElementById('content');
  if (editorEl) {
    const endpoint = editorEl.getAttribute('data-upload-endpoint') || '/api/uploads';
    new EditorMediaUpload(editorEl, endpoint);
  }
});
