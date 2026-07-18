class DocumentAutosave {
  constructor(options = {}) {
    this.formEl = options.formEl || document.querySelector('form.editor');
    this.editorEl = options.editorEl;
    this.titleEl = options.titleEl;
    this.indicatorEl = options.indicatorEl;
    this.docId = Number(options.docId || 0);
    this.lockVersion = Number(options.lockVersion || 1);
    this.debounceMs = Number(options.debounceMs || 900);
    this.state = 'idle';
    this.saveTimeout = 0;
    this.inFlight = null;
    this.changedWhileSaving = false;
    this.submitting = false;
    this.lastSavedSnapshot = '';
    this.boundFields = [];
    this.init();
  }

  init() {
    if (!this.formEl || !this.editorEl) return;
    this.boundFields = Array.from(this.formEl.querySelectorAll(
      '[name="title"],[name="slug"],[name="content"],[name="space_id"],[name="category_id"],[name="visibility"],[name="classification"],[name="language"]'
    ));
    this.boundFields.forEach((field) => {
      field.addEventListener(field.matches('select') ? 'change' : 'input', () => this.onInput());
    });
    this.formEl.addEventListener('submit', (event) => this.onSubmit(event));
    window.addEventListener('beforeunload', (event) => {
      if (this.state !== 'dirty' && this.state !== 'saving') return;
      event.preventDefault();
      event.returnValue = '';
    });
    this.updateWordCount();
    this.lastSavedSnapshot = this.snapshotString();
    this.checkLocalRecovery();
  }

  field(name) {
    return this.formEl.elements.namedItem(name);
  }

  snapshot() {
    return {
      id: this.docId,
      title: String(this.titleEl?.value || ''),
      slug: String(this.field('slug')?.value || ''),
      content: String(this.editorEl.value || ''),
      space_id: Number(this.field('space_id')?.value || 1),
      category_id: Number(this.field('category_id')?.value || 0),
      visibility: String(this.field('visibility')?.value || 'authenticated'),
      classification: String(this.field('classification')?.value || 'internal'),
      language: String(this.field('language')?.value || 'ru'),
      lock_version: this.lockVersion,
    };
  }

  snapshotString(payload = this.snapshot()) {
    const comparable = { ...payload };
    delete comparable.lock_version;
    return JSON.stringify(comparable);
  }

  onInput() {
    this.updateWordCount();
    const payload = this.snapshot();
    if (this.snapshotString(payload) === this.lastSavedSnapshot) return;
    this.persistLocal(payload);
    if (this.state === 'saving') {
      this.changedWhileSaving = true;
      return;
    }
    this.setState('dirty');
    window.clearTimeout(this.saveTimeout);
    this.saveTimeout = window.setTimeout(() => this.performAutosave(), this.debounceMs);
  }

  async onSubmit(event) {
    if (this.submitting || (this.state !== 'dirty' && this.state !== 'saving')) return;
    event.preventDefault();
    const submitter = event.submitter;
    const canSubmit = await this.flush();
    if (!canSubmit) return;
    this.submitting = true;
    if (submitter && this.formEl.requestSubmit) this.formEl.requestSubmit(submitter);
    else this.formEl.submit();
  }

  async flush() {
    window.clearTimeout(this.saveTimeout);
    if (this.inFlight) await this.inFlight;
    if (this.state === 'dirty') await this.performAutosave();
    return this.state !== 'conflict';
  }

  setState(nextState, message = '') {
    this.state = nextState;
    if (!this.indicatorEl) return;
    const labels = {
      idle: '',
      dirty: 'Есть несохранённые изменения',
      saving: 'Сохраняем черновик…',
      saved: message || 'Все изменения сохранены',
      error: message || 'Копия сохранена локально',
      conflict: 'Обнаружен конфликт версий',
    };
    this.indicatorEl.textContent = labels[nextState] || message;
    this.indicatorEl.dataset.state = nextState;
  }

  performAutosave() {
    if (this.inFlight) return this.inFlight;
    const payload = this.snapshot();
    const requestSnapshot = this.snapshotString(payload);
    if (requestSnapshot === this.lastSavedSnapshot) {
      this.setState('saved');
      return Promise.resolve(true);
    }
    this.persistLocal(payload);
    this.changedWhileSaving = false;
    this.setState('saving');
    this.inFlight = this.send(payload, requestSnapshot).finally(() => {
      this.inFlight = null;
      if (this.changedWhileSaving || this.snapshotString() !== this.lastSavedSnapshot) {
        this.setState('dirty');
        window.clearTimeout(this.saveTimeout);
        this.saveTimeout = window.setTimeout(() => this.performAutosave(), 250);
      }
    });
    return this.inFlight;
  }

  async send(payload, requestSnapshot) {
    try {
      const csrf = document.querySelector('meta[name="csrf-token"]')?.content || '';
      const response = await fetch('/api/v1/documents/draft', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
        body: JSON.stringify(payload),
      });
      const data = await response.json().catch(() => ({}));
      if (response.status === 409) {
        this.setState('conflict');
        this.showConflict(data);
        window.dispatchEvent(new CustomEvent('dh:edit-conflict', { detail: data }));
        return false;
      }
      if (!response.ok) throw new Error(data.message || `HTTP ${response.status}`);

      const previousKey = this.storageKey();
      const wasNew = this.docId === 0;
      if (data.id) {
        this.docId = Number(data.id);
        const idField = this.field('id');
        if (idField) idField.value = String(this.docId);
      }
      if (data.lock_version) {
        this.lockVersion = Number(data.lock_version);
        const lockField = this.field('lock_version');
        if (lockField) lockField.value = String(this.lockVersion);
      }
      const slugField = this.field('slug');
      if (data.slug && slugField && !slugField.value) slugField.value = data.slug;
      if (wasNew && data.slug) {
        history.replaceState({}, '', `/edit/${encodeURIComponent(data.slug)}`);
        const picker = document.querySelector('.template-picker');
        if (picker) picker.hidden = true;
      }
      localStorage.removeItem(previousKey);
      localStorage.removeItem(this.storageKey());
      this.lastSavedSnapshot = this.snapshotString({
        ...payload,
        id: this.docId,
        slug: data.slug || payload.slug,
        lock_version: this.lockVersion,
      });
      if (this.snapshotString() === this.lastSavedSnapshot) {
        const time = new Date().toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
        this.setState('saved', `Сохранено в ${time}`);
      }
      window.dispatchEvent(new CustomEvent('dh:autosave-complete', { detail: data }));
      return true;
    } catch (error) {
      console.warn('Autosave failed; local recovery copy retained.', error);
      this.setState('error', 'Нет связи — копия сохранена локально');
      return false;
    }
  }

  storageKey() {
    return `dh_draft_${this.docId || 'new'}`;
  }

  persistLocal(payload) {
    try {
      localStorage.setItem(this.storageKey(), JSON.stringify({ ...payload, saved_locally_at: new Date().toISOString() }));
    } catch (error) {
      console.warn('Local recovery storage is unavailable.', error);
    }
  }

  checkLocalRecovery() {
    let saved;
    try { saved = localStorage.getItem(this.storageKey()); } catch (_) { return; }
    if (!saved) return;
    try {
      const draft = JSON.parse(saved);
      if (!draft.content || draft.content === this.editorEl.value) return;
      if (window.confirm('Найдена более новая локальная копия. Восстановить её?')) {
        this.editorEl.value = draft.content;
        if (this.titleEl && draft.title) this.titleEl.value = draft.title;
        ['slug', 'space_id', 'category_id', 'visibility', 'classification', 'language'].forEach((name) => {
          const control = this.field(name);
          if (control && draft[name] !== undefined) control.value = String(draft[name]);
        });
        this.onInput();
      } else {
        localStorage.removeItem(this.storageKey());
      }
    } catch (error) {
      localStorage.removeItem(this.storageKey());
    }
  }

  updateWordCount() {
    const counter = document.getElementById('editorWordCount');
    if (!counter) return;
    const words = (this.editorEl.value.trim().match(/[\p{L}\p{N}_-]+/gu) || []).length;
    counter.textContent = `${words} ${words === 1 ? 'слово' : 'слов'}`;
  }

  showConflict(data) {
    if (document.getElementById('autosaveConflictOverlay')) return;
    const overlay = document.createElement('div');
    overlay.id = 'autosaveConflictOverlay';
    overlay.className = 'dialog-overlay open conflict-dialog';
    overlay.innerHTML = `
      <section class="dialog-content" role="alertdialog" aria-modal="true" aria-labelledby="conflictTitle" aria-describedby="conflictDescription">
        <header class="dialog-header"><h2 class="dialog-title" id="conflictTitle">Документ изменён в другой вкладке</h2></header>
        <div class="dialog-body"><p id="conflictDescription">Чтобы не потерять чужие изменения, автоматическое сохранение остановлено. Скопируйте свой текст или загрузите актуальную версию.</p><p>Версия на сервере: <strong>${Number(data.server_lock_version || 0)}</strong></p></div>
        <footer class="dialog-footer"><button class="btn btn-secondary" type="button" data-copy>Скопировать мой текст</button><button class="btn btn-primary" type="button" data-reload>Загрузить актуальную версию</button></footer>
      </section>`;
    document.body.appendChild(overlay);
    const reload = overlay.querySelector('[data-reload]');
    const copy = overlay.querySelector('[data-copy]');
    reload.addEventListener('click', () => window.location.reload());
    copy.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(this.editorEl.value);
        copy.textContent = 'Текст скопирован';
      } catch (_) {
        this.editorEl.focus();
        this.editorEl.select();
      }
    });
    reload.focus();
  }
}

window.DocumentAutosave = DocumentAutosave;
