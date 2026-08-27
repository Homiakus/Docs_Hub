/**
 * Docs_Hub — Anchored Comments & Collaborative Review Module
 */
class DocumentComments {
  constructor(options = {}) {
    this.docId = Number(options.docId || 0);
    this.canComment = options.canComment === true || options.canComment === 'true';
    this.csrfToken = document.querySelector('meta[name="csrf-token"]')?.getAttribute('content') || '';
    this.container = document.getElementById('commentsSection');
    this.listEl = document.getElementById('commentsList');
    this.countEl = document.getElementById('commentCount');
    this.generalForm = document.getElementById('generalCommentForm');
    this.generalInput = document.getElementById('generalCommentInput');
    this.markdownEl = document.querySelector('.reader-page .markdown');
    
    this.comments = [];
    this.currentFilter = 'all';
    this.selectedAnchor = null;
    this.tooltipEl = null;

    if (this.docId > 0 && this.listEl) {
      this.init();
    }
  }

  async init() {
    this.createFloatingTooltip();
    this.bindEvents();
    await this.loadComments();
  }

  createFloatingTooltip() {
    if (!this.canComment || !this.markdownEl) return;
    this.tooltipEl = document.createElement('div');
    this.tooltipEl.className = 'selection-comment-tooltip';
    this.tooltipEl.innerHTML = `
      <button type="button" class="btn btn-sm btn-primary" id="btnSelectionComment">
        <svg class="icon icon-xs" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
        <span>Комментировать</span>
      </button>
    `;
    this.tooltipEl.style.display = 'none';
    document.body.appendChild(this.tooltipEl);

    this.tooltipEl.querySelector('#btnSelectionComment')?.addEventListener('click', (e) => {
      e.stopPropagation();
      this.promptSelectionComment();
    });
  }

  bindEvents() {
    // Filter tabs
    this.container?.querySelectorAll('[data-comment-filter]').forEach(btn => {
      btn.addEventListener('click', () => {
        this.container.querySelectorAll('[data-comment-filter]').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        this.currentFilter = btn.getAttribute('data-comment-filter') || 'all';
        this.renderComments();
      });
    });

    // General document comment form
    if (this.generalForm) {
      this.generalForm.addEventListener('submit', (e) => {
        e.preventDefault();
        const text = (this.generalInput?.value || '').trim();
        if (!text) return;
        this.submitComment({ body: text });
      });
    }

    // Text selection inside markdown container
    if (this.markdownEl && this.canComment) {
      document.addEventListener('selectionchange', () => this.handleSelectionChange());
      document.addEventListener('mouseup', () => this.handleSelectionChange());
    }
  }

  handleSelectionChange() {
    if (!this.tooltipEl || !this.markdownEl) return;
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || !selection.rangeCount) {
      this.hideTooltip();
      return;
    }

    const range = selection.getRangeAt(0);
    if (!this.markdownEl.contains(range.commonAncestorContainer)) {
      this.hideTooltip();
      return;
    }

    const text = selection.toString().trim();
    if (text.length < 2) {
      this.hideTooltip();
      return;
    }

    const rect = range.getBoundingClientRect();
    if (rect.width === 0 || rect.height === 0) {
      this.hideTooltip();
      return;
    }

    // Calculate anchor details
    const fullText = this.markdownEl.innerText || '';
    const startIndex = Math.max(0, fullText.indexOf(text));
    const prefix = fullText.substring(Math.max(0, startIndex - 32), startIndex);
    const suffix = fullText.substring(startIndex + text.length, startIndex + text.length + 32);

    this.selectedAnchor = {
      quote_exact: text,
      quote_prefix: prefix,
      quote_suffix: suffix,
      start_offset: startIndex,
      end_offset: startIndex + text.length,
    };

    this.tooltipEl.style.display = 'block';
    this.tooltipEl.style.position = 'absolute';
    this.tooltipEl.style.left = `${window.scrollX + rect.left + (rect.width / 2) - 60}px`;
    this.tooltipEl.style.top = `${window.scrollY + rect.top - 42}px`;
  }

  hideTooltip() {
    if (this.tooltipEl) {
      this.tooltipEl.style.display = 'none';
    }
  }

  promptSelectionComment() {
    if (!this.selectedAnchor) return;
    const quote = this.selectedAnchor.quote_exact;
    const body = window.prompt(`Комментарий к фрагменту:\n"${quote.length > 60 ? quote.substring(0, 57) + '...' : quote}"`);
    if (body && body.trim()) {
      this.submitComment({
        ...this.selectedAnchor,
        body: body.trim()
      });
    }
    this.hideTooltip();
    window.getSelection()?.removeAllRanges();
  }

  async loadComments() {
    try {
      const res = await fetch(`/api/v1/documents/${this.docId}/comments`, {
        headers: { 'Accept': 'application/json' }
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      this.comments = Array.isArray(data.comments) ? data.comments : [];
      this.updateCount();
      this.renderComments();
      this.renderHighlights();
    } catch (err) {
      if (this.listEl) {
        this.listEl.innerHTML = `<p class="u-muted">Не удалось загрузить комментарии.</p>`;
      }
    }
  }

  updateCount() {
    if (this.countEl) {
      this.countEl.textContent = String(this.comments.length);
    }
  }

  renderComments() {
    if (!this.listEl) return;
    const filtered = this.comments.filter(c => {
      if (this.currentFilter === 'open') return c.status === 'open';
      if (this.currentFilter === 'resolved') return c.status === 'resolved';
      return true;
    });

    if (filtered.length === 0) {
      this.listEl.innerHTML = `<p class="u-muted" style="padding: 1rem 0;">${this.comments.length === 0 ? 'Комментариев пока нет. Выделите текст или напишите отзыв.' : 'Нет комментариев по выбранному фильтру.'}</p>`;
      return;
    }

    // Group threads
    const rootComments = filtered.filter(c => !c.parent_id);
    const repliesMap = new Map();
    filtered.filter(c => c.parent_id).forEach(reply => {
      const arr = repliesMap.get(reply.parent_id) || [];
      arr.push(reply);
      repliesMap.set(reply.parent_id, arr);
    });

    this.listEl.innerHTML = rootComments.map(c => this.renderThreadHTML(c, repliesMap.get(c.id) || [])).join('');
    this.bindThreadActions();
  }

  renderThreadHTML(comment, replies) {
    const isOpen = comment.status === 'open';
    const hasQuote = Boolean(comment.quote_exact);
    const dateStr = comment.created_at ? new Date(comment.created_at).toLocaleString('ru', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '';
    
    return `
      <article class="comment-card ${isOpen ? 'status-open' : 'status-resolved'}" data-comment-id="${comment.id}">
        <header class="comment-card-head">
          <div class="comment-author">
            <span class="avatar-dot" aria-hidden="true">${(comment.author_name || 'U').charAt(0).toUpperCase()}</span>
            <strong>${this.escapeHTML(comment.author_name || 'Пользователь')}</strong>
          </div>
          <div class="comment-meta">
            <time>${dateStr}</time>
            <span class="status-badge status-badge-${comment.status}">${isOpen ? 'Открыт' : 'Решён'}</span>
          </div>
        </header>

        ${hasQuote ? `
          <blockquote class="comment-quote">
            <svg class="icon icon-xs" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 21c3 0 7-1 7-8V5c0-1.25-.75-2-2-2H4c-1.25 0-2 .75-2 2v6c0 1.25.75 2 2 2 0 4-1 6-1 8zm14 0c3 0 7-1 7-8V5c0-1.25-.75-2-2-2h-4c-1.25 0-2 .75-2 2v6c0 1.25.75 2 2 2 0 4-1 6-1 8z"/></svg>
            <span>${this.escapeHTML(comment.quote_exact)}</span>
          </blockquote>
        ` : ''}

        <div class="comment-body">${this.escapeHTML(comment.body)}</div>

        <div class="comment-actions">
          ${isOpen ? `
            <button type="button" class="btn btn-ghost btn-xs" data-action="resolve" data-id="${comment.id}">
              <svg class="icon icon-xs" viewBox="0 0 24 24" aria-hidden="true"><polyline points="20 6 9 17 4 12"/></svg>
              <span>Решить</span>
            </button>
          ` : ''}
          ${this.canComment ? `
            <button type="button" class="btn btn-ghost btn-xs" data-action="reply-toggle" data-id="${comment.id}">
              <svg class="icon icon-xs" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
              <span>Ответить</span>
            </button>
          ` : ''}
          <button type="button" class="btn btn-ghost btn-xs btn-danger-hover" data-action="delete" data-id="${comment.id}" title="Удалить">
            <svg class="icon icon-xs" viewBox="0 0 24 24" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>

        ${replies.length > 0 ? `
          <div class="comment-replies">
            ${replies.map(r => this.renderReplyHTML(r)).join('')}
          </div>
        ` : ''}

        ${this.canComment ? `
          <form class="comment-reply-form" data-parent-id="${comment.id}" style="display: none;">
            <input type="text" class="form-control form-control-sm" placeholder="Ответить в ветке…" required autocomplete="off">
            <button type="submit" class="btn btn-secondary btn-xs">Отправить</button>
          </form>
        ` : ''}
      </article>
    `;
  }

  renderReplyHTML(reply) {
    const dateStr = reply.created_at ? new Date(reply.created_at).toLocaleString('ru', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '';
    return `
      <div class="comment-reply-item" data-comment-id="${reply.id}">
        <header class="comment-card-head">
          <div class="comment-author">
            <span class="avatar-dot" aria-hidden="true">${(reply.author_name || 'U').charAt(0).toUpperCase()}</span>
            <strong>${this.escapeHTML(reply.author_name || 'Пользователь')}</strong>
          </div>
          <time>${dateStr}</time>
          <button type="button" class="btn btn-ghost btn-xs btn-danger-hover" data-action="delete" data-id="${reply.id}" title="Удалить ответ">
            <svg class="icon icon-xs" viewBox="0 0 24 24" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </header>
        <div class="comment-body">${this.escapeHTML(reply.body)}</div>
      </div>
    `;
  }

  bindThreadActions() {
    if (!this.listEl) return;

    // Resolve
    this.listEl.querySelectorAll('[data-action="resolve"]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const id = btn.getAttribute('data-id');
        if (!id) return;
        await this.resolveComment(id);
      });
    });

    // Delete
    this.listEl.querySelectorAll('[data-action="delete"]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const id = btn.getAttribute('data-id');
        if (!id) return;
        if (confirm('Удалить этот комментарий?')) {
          await this.deleteComment(id);
        }
      });
    });

    // Reply Toggle
    this.listEl.querySelectorAll('[data-action="reply-toggle"]').forEach(btn => {
      btn.addEventListener('click', () => {
        const id = btn.getAttribute('data-id');
        const form = this.listEl.querySelector(`form[data-parent-id="${id}"]`);
        if (form) {
          form.style.display = form.style.display === 'none' ? 'flex' : 'none';
          form.querySelector('input')?.focus();
        }
      });
    });

    // Reply Submit
    this.listEl.querySelectorAll('.comment-reply-form').forEach(form => {
      form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const parentId = Number(form.getAttribute('data-parent-id'));
        const input = form.querySelector('input');
        const body = (input?.value || '').trim();
        if (!body || !parentId) return;
        await this.submitComment({
          parent_id: parentId,
          body: body
        });
      });
    });
  }

  async submitComment(payload) {
    try {
      const res = await fetch(`/api/v1/documents/${this.docId}/comments`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': this.csrfToken,
          'Accept': 'application/json'
        },
        body: JSON.stringify(payload)
      });

      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.message || `HTTP ${res.status}`);
      }

      if (this.generalInput) this.generalInput.value = '';
      if (window.ToastManager) {
        window.ToastManager.show('success', 'Комментарий добавлен');
      }
      await this.loadComments();
    } catch (err) {
      alert(`Ошибка отправки комментария: ${err.message}`);
    }
  }

  async resolveComment(commentId) {
    try {
      const res = await fetch(`/api/v1/comments/${commentId}/resolve`, {
        method: 'POST',
        headers: {
          'X-CSRF-Token': this.csrfToken,
          'Accept': 'application/json'
        }
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      if (window.ToastManager) {
        window.ToastManager.show('success', 'Ветка отмечена как решённая');
      }
      await this.loadComments();
    } catch (err) {
      alert(`Ошибка: ${err.message}`);
    }
  }

  async deleteComment(commentId) {
    try {
      const res = await fetch(`/api/v1/comments/${commentId}`, {
        method: 'DELETE',
        headers: {
          'X-CSRF-Token': this.csrfToken,
          'Accept': 'application/json'
        }
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      if (window.ToastManager) {
        window.ToastManager.show('info', 'Комментарий удалён');
      }
      await this.loadComments();
    } catch (err) {
      alert(`Ошибка удаления: ${err.message}`);
    }
  }

  renderHighlights() {
    if (!this.markdownEl) return;
    // Clean existing comment highlights
    this.markdownEl.querySelectorAll('.comment-highlight').forEach(el => {
      const parent = el.parentNode;
      while (el.firstChild) parent.insertBefore(el.firstChild, el);
      parent.removeChild(el);
    });

    const openQuotes = this.comments
      .filter(c => c.status === 'open' && c.quote_exact && c.quote_exact.length > 2)
      .map(c => ({ id: c.id, text: c.quote_exact }));

    if (openQuotes.length === 0) return;

    // Simple text node annotator for exact quotes
    const walker = document.createTreeWalker(this.markdownEl, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);

    openQuotes.forEach(q => {
      for (const node of nodes) {
        if (!node.parentNode || node.parentNode.classList?.contains('comment-highlight') || node.parentNode.nodeName === 'SCRIPT') continue;
        const idx = node.nodeValue.indexOf(q.text);
        if (idx !== -1) {
          const span = document.createElement('mark');
          span.className = 'comment-highlight';
          span.setAttribute('data-comment-ref', String(q.id));
          span.title = 'Нажмите, чтобы перейти к комментарию';

          const textBefore = node.nodeValue.substring(0, idx);
          const matchedText = node.nodeValue.substring(idx, idx + q.text.length);
          const textAfter = node.nodeValue.substring(idx + q.text.length);

          node.nodeValue = textBefore;
          span.textContent = matchedText;
          const afterNode = document.createTextNode(textAfter);

          const parent = node.parentNode;
          parent.insertBefore(span, node.nextSibling);
          parent.insertBefore(afterNode, span.nextSibling);

          span.addEventListener('click', () => {
            const card = this.listEl?.querySelector(`[data-comment-id="${q.id}"]`);
            if (card) {
              card.scrollIntoView({ behavior: 'smooth', block: 'center' });
              card.classList.add('highlight-pulse');
              setTimeout(() => card.classList.remove('highlight-pulse'), 1800);
            }
          });
          break;
        }
      }
    });
  }

  escapeHTML(str) {
    return String(str || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }
}

window.DocumentComments = DocumentComments;

document.addEventListener('DOMContentLoaded', () => {
  const section = document.getElementById('commentsSection');
  if (section) {
    const docId = section.getAttribute('data-document-id');
    const canComment = section.getAttribute('data-can-comment');
    new DocumentComments({ docId, canComment });
  }
});
