/* Autosave & Draft Recovery State Machine Module */

class DocumentAutosave {
  constructor(options = {}) {
    this.editorEl = options.editorEl;
    this.titleEl = options.titleEl;
    this.docId = options.docId || 0;
    this.indicatorEl = options.indicatorEl;
    this.lockVersion = options.lockVersion || 1;
    this.debounceMs = options.debounceMs || 1000;
    this.saveTimeout = null;
    this.state = 'idle'; // idle | dirty | saving | saved | error | conflict

    this.init();
  }

  init() {
    if (!this.editorEl) return;

    this.editorEl.addEventListener('input', () => this.onInput());
    if (this.titleEl) {
      this.titleEl.addEventListener('input', () => this.onInput());
    }

    // Restore local recovery draft if present
    this.checkLocalRecovery();
  }

  onInput() {
    this.setState('dirty');
    clearTimeout(this.saveTimeout);
    this.saveTimeout = setTimeout(() => this.performAutosave(), this.debounceMs);
  }

  setState(newState, message) {
    this.state = newState;
    if (!this.indicatorEl) return;

    switch (newState) {
      case 'dirty':
        this.indicatorEl.textContent = 'Есть несохраненные изменения...';
        this.indicatorEl.style.color = 'var(--text-tertiary)';
        break;
      case 'saving':
        this.indicatorEl.textContent = 'Сохранение черновика...';
        this.indicatorEl.style.color = 'var(--action-primary)';
        break;
      case 'saved':
        this.indicatorEl.textContent = 'Черновик сохранен';
        this.indicatorEl.style.color = 'var(--status-success)';
        break;
      case 'error':
        this.indicatorEl.textContent = message || 'Ошибка автосохранения';
        this.indicatorEl.style.color = 'var(--status-danger)';
        break;
      case 'conflict':
        this.indicatorEl.textContent = '⚠️ Конфликт версий!';
        this.indicatorEl.style.color = 'var(--status-warning)';
        break;
      default:
        this.indicatorEl.textContent = '';
    }
  }

  async performAutosave() {
    if (this.state === 'saving') return;
    this.setState('saving');

    const payload = {
      id: this.docId,
      title: this.titleEl ? this.titleEl.value : '',
      content: this.editorEl.value,
      lock_version: this.lockVersion,
    };

    // Store local fallback copy in localStorage
    localStorage.setItem(`dh_draft_${this.docId || 'new'}`, JSON.stringify(payload));

    try {
      const csrfMeta = document.querySelector('meta[name="csrf-token"]');
      const csrf = csrfMeta ? csrfMeta.getAttribute('content') : '';

      const resp = await fetch('/api/v1/documents/draft', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': csrf,
        },
        body: JSON.stringify(payload),
      });

      if (resp.status === 409) {
        this.setState('conflict');
        window.dispatchEvent(new CustomEvent('dh:edit-conflict', { detail: await resp.json() }));
        return;
      }

      if (!resp.ok) {
        throw new Error(`Server returned HTTP ${resp.status}`);
      }

      const res = await resp.json();
      if (res.lock_version) this.lockVersion = res.lock_version;
      this.setState('saved');

      // Clear local recovery draft on successful sync
      localStorage.removeItem(`dh_draft_${this.docId || 'new'}`);
    } catch (err) {
      console.warn('Autosave sync failed, draft saved locally:', err);
      this.setState('error', 'Автосохранение в автономном режиме');
    }
  }

  checkLocalRecovery() {
    const key = `dh_draft_${this.docId || 'new'}`;
    const saved = localStorage.getItem(key);
    if (saved) {
      try {
        const draft = JSON.parse(saved);
        if (draft.content && draft.content !== this.editorEl.value) {
          if (confirm('Обнаружен несохраненный локальный черновик. Восстановить?')) {
            this.editorEl.value = draft.content;
            if (this.titleEl && draft.title) this.titleEl.value = draft.title;
            this.editorEl.dispatchEvent(new Event('input', { bubbles: true }));
          } else {
            localStorage.removeItem(key);
          }
        }
      } catch (e) {
        console.error('Failed to parse local draft:', e);
      }
    }
  }
}

window.DocumentAutosave = DocumentAutosave;
