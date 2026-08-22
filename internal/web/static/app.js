/**
 * Docs_Hub Client Core Application
 * Architecture: State → Feedback → Motion → Color → Meaning
 */
(() => {
  'use strict';

  // ---------------------------------------------------------------------------
  // 01. UTILITIES & HELPERS
  // ---------------------------------------------------------------------------
  const debounce = (fn, delay) => {
    let timer;
    return (...args) => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => fn(...args), delay);
    };
  };

  const csrfToken = () => document.querySelector('meta[name="csrf-token"]')?.content || '';

  const apiFetch = (url, options = {}) => {
    const headers = new Headers(options.headers || {});
    if (csrfToken() && !headers.has('X-CSRF-Token')) {
      headers.set('X-CSRF-Token', csrfToken());
    }
    return fetch(url, { ...options, headers });
  };

  // Color math helper for custom accent generation
  function hexToRgb(hex) {
    let c = hex.replace(/^#/, '');
    if (c.length === 3) c = c.split('').map(x => x + x).join('');
    const num = parseInt(c, 16);
    return {
      r: (num >> 16) & 255,
      g: (num >> 8) & 255,
      b: num & 255,
    };
  }

  function rgbToHex(r, g, b) {
    const clamp = (v) => Math.max(0, Math.min(255, Math.round(v)));
    return '#' + [r, g, b].map(v => clamp(v).toString(16).padStart(2, '0')).join('');
  }

  function adjustBrightness(hex, percent) {
    const { r, g, b } = hexToRgb(hex);
    const factor = 1 + percent / 100;
    return rgbToHex(r * factor, g * factor, b * factor);
  }

  function getLuminance(hex) {
    const { r, g, b } = hexToRgb(hex);
    const a = [r, g, b].map((v) => {
      v /= 255;
      return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
    });
    return a[0] * 0.2126 + a[1] * 0.7152 + a[2] * 0.0722;
  }

  // ---------------------------------------------------------------------------
  // 02. TOAST NOTIFICATION MANAGER
  // ---------------------------------------------------------------------------
  const ToastManager = {
    region: null,

    getRegion() {
      if (!this.region || !document.body.contains(this.region)) {
        this.region = document.querySelector('.toast-region');
        if (!this.region) {
          this.region = document.createElement('div');
          this.region.className = 'toast-region';
          this.region.setAttribute('aria-live', 'polite');
          this.region.setAttribute('aria-atomic', 'false');
          document.body.appendChild(this.region);
        }
      }
      return this.region;
    },

    getIconSvg(type) {
      switch (type) {
        case 'success':
          return '<svg class="icon icon-sm icon-check-animated" viewBox="0 0 24 24"><path d="M20 6L9 17l-5-5"/></svg>';
        case 'warning':
          return '<svg class="icon icon-sm" viewBox="0 0 24 24"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>';
        case 'error':
          return '<svg class="icon icon-sm" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>';
        case 'progress':
          return '<svg class="icon icon-sm icon-spin" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="12"/></svg>';
        case 'info':
        default:
          return '<svg class="icon icon-sm" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>';
      }
    },

    show(message, type = 'info', duration = 3400) {
      const region = this.getRegion();
      const toast = document.createElement('div');
      toast.className = `toast toast-${type}`;
      toast.setAttribute('role', type === 'error' ? 'alert' : 'status');

      toast.innerHTML = `
        <span class="toast-icon" aria-hidden="true">${this.getIconSvg(type)}</span>
        <span class="toast-message">${message}</span>
        <button class="toast-close" type="button" aria-label="Закрыть уведомление">
          <svg class="icon icon-xs" viewBox="0 0 24 24"><path d="M18 6L6 18M6 6l12 12"/></svg>
        </button>
      `;

      const closeBtn = toast.querySelector('.toast-close');
      let dismissTimer = null;

      const dismiss = () => {
        if (dismissTimer) window.clearTimeout(dismissTimer);
        toast.classList.add('is-hiding');
        window.setTimeout(() => {
          toast.remove();
        }, 220);
      };

      closeBtn?.addEventListener('click', dismiss);

      if (duration > 0) {
        dismissTimer = window.setTimeout(dismiss, duration);
      }

      region.appendChild(toast);
      return { dismiss };
    }
  };

  // Expose global showToast helper
  window.showToast = (msg, type = 'info', duration = 3400) => ToastManager.show(msg, type, duration);

  // ---------------------------------------------------------------------------
  // 03. APPEARANCE & MOTION SETTINGS MANAGER
  // ---------------------------------------------------------------------------
  const AppearanceManager = {
    modal: null,
    previousFocus: null,
    systemDarkMedia: window.matchMedia('(prefers-color-scheme: dark)'),

    state: {
      theme: 'system', // 'system' | 'light' | 'dark'
      accent: 'indigo', // preset name or 'custom'
      customColor: '#2152ff',
      density: 'comfortable', // 'comfortable' | 'compact'
      motion: 'system', // 'system' | 'full' | 'reduced'
    },

    init() {
      this.modal = document.getElementById('appearanceModal');
      this.loadPreferences();
      this.applyAll();
      this.bindEvents();
    },

    loadPreferences() {
      try {
        const storedTheme = localStorage.getItem('docs-hub-theme');
        if (storedTheme) this.state.theme = storedTheme;

        const storedAccent = localStorage.getItem('docs-hub-accent');
        if (storedAccent) this.state.accent = storedAccent;

        const storedCustom = localStorage.getItem('docs-hub-custom-accent');
        if (storedCustom) this.state.customColor = storedCustom;

        const storedDensity = localStorage.getItem('docs-hub-density');
        if (storedDensity) this.state.density = storedDensity;

        const storedMotion = localStorage.getItem('docs-hub-motion');
        if (storedMotion) this.state.motion = storedMotion;
      } catch (_) {}
    },

    savePreferences() {
      try {
        localStorage.setItem('docs-hub-theme', this.state.theme);
        localStorage.setItem('docs-hub-accent', this.state.accent);
        localStorage.setItem('docs-hub-custom-accent', this.state.customColor);
        localStorage.setItem('docs-hub-density', this.state.density);
        localStorage.setItem('docs-hub-motion', this.state.motion);
      } catch (_) {}
    },

    applyTheme() {
      const isDark = this.state.theme === 'dark' || (this.state.theme === 'system' && this.systemDarkMedia.matches);
      const effectiveTheme = isDark ? 'dark' : 'light';
      document.documentElement.dataset.theme = effectiveTheme;
      document.documentElement.dataset.themePreference = this.state.theme;

      // Update meta theme-color
      const themeColorMeta = document.querySelector('meta[name="theme-color"]');
      if (themeColorMeta) {
        themeColorMeta.content = isDark ? '#0c0d11' : '#f7f7f4';
      }

      // Re-apply custom accent if custom is active (to adapt dark/light shades)
      if (this.state.accent === 'custom') {
        this.applyCustomAccent(this.state.customColor);
      }

      // Update mermaid theme if present
      if (window.mermaid) {
        try {
          window.mermaid.initialize({
            startOnLoad: false,
            theme: isDark ? 'dark' : 'neutral',
            securityLevel: 'strict'
          });
        } catch (_) {}
      }
    },

    applyAccent() {
      if (this.state.accent === 'custom') {
        document.documentElement.dataset.accent = 'custom';
        this.applyCustomAccent(this.state.customColor);
      } else {
        this.clearCustomAccentProperties();
        document.documentElement.dataset.accent = this.state.accent;
      }
    },

    applyCustomAccent(hex) {
      const isDark = document.documentElement.dataset.theme === 'dark';
      const rootStyle = document.documentElement.style;
      const { r, g, b } = hexToRgb(hex);

      const hoverHex = isDark ? adjustBrightness(hex, 18) : adjustBrightness(hex, -14);
      const activeHex = isDark ? adjustBrightness(hex, 32) : adjustBrightness(hex, -24);
      const subtleBg = isDark
        ? `rgba(${r}, ${g}, ${b}, 0.15)`
        : `rgba(${r}, ${g}, ${b}, 0.08)`;
      const mutedBg = `rgba(${r}, ${g}, ${b}, 0.14)`;
      const borderCol = `rgba(${r}, ${g}, ${b}, 0.32)`;
      const contrast = getLuminance(hex) > 0.45 ? '#0c0d11' : '#ffffff';

      rootStyle.setProperty('--accent', hex);
      rootStyle.setProperty('--accent-hover', hoverHex);
      rootStyle.setProperty('--accent-active', activeHex);
      rootStyle.setProperty('--accent-subtle', subtleBg);
      rootStyle.setProperty('--accent-muted', mutedBg);
      rootStyle.setProperty('--accent-border', borderCol);
      rootStyle.setProperty('--accent-contrast', contrast);
      rootStyle.setProperty('--focus-ring', `0 0 0 1px var(--surface-primary), 0 0 0 3px ${hex}`);
    },

    clearCustomAccentProperties() {
      const rootStyle = document.documentElement.style;
      rootStyle.removeProperty('--accent');
      rootStyle.removeProperty('--accent-hover');
      rootStyle.removeProperty('--accent-active');
      rootStyle.removeProperty('--accent-subtle');
      rootStyle.removeProperty('--accent-muted');
      rootStyle.removeProperty('--accent-border');
      rootStyle.removeProperty('--accent-contrast');
      rootStyle.removeProperty('--focus-ring');
    },

    applyDensity() {
      document.documentElement.dataset.density = this.state.density;
    },

    applyMotion() {
      document.documentElement.dataset.motion = this.state.motion;
    },

    applyAll() {
      this.applyTheme();
      this.applyAccent();
      this.applyDensity();
      this.applyMotion();
      this.syncControls();
    },

    syncControls() {
      if (!this.modal) return;

      // Theme radio cards
      this.modal.querySelectorAll('[data-theme-choice]').forEach((btn) => {
        const selected = btn.dataset.themeChoice === this.state.theme;
        btn.setAttribute('aria-checked', String(selected));
        btn.classList.toggle('selected', selected);
      });

      // Accent swatches
      this.modal.querySelectorAll('[data-accent-choice]').forEach((btn) => {
        const selected = btn.dataset.accentChoice === this.state.accent;
        btn.setAttribute('aria-checked', String(selected));
        btn.classList.toggle('selected', selected);
      });

      // Custom accent input
      const customInput = this.modal.querySelector('#customAccentInput');
      const customValue = this.modal.querySelector('#customAccentValue');
      if (customInput) customInput.value = this.state.customColor;
      if (customValue) customValue.textContent = this.state.customColor.toUpperCase();

      // Density segmented buttons
      this.modal.querySelectorAll('[data-density-choice]').forEach((btn) => {
        const selected = btn.dataset.densityChoice === this.state.density;
        btn.setAttribute('aria-checked', String(selected));
        btn.classList.toggle('selected', selected);
      });

      // Motion segmented buttons
      this.modal.querySelectorAll('[data-motion-choice]').forEach((btn) => {
        const selected = btn.dataset.motionChoice === this.state.motion;
        btn.setAttribute('aria-checked', String(selected));
        btn.classList.toggle('selected', selected);
      });
    },

    openModal() {
      if (!this.modal) return;
      this.previousFocus = document.activeElement;
      this.syncControls();
      this.modal.setAttribute('aria-hidden', 'false');
      this.modal.classList.add('open');
      document.body.classList.add('modal-open');

      const firstChoice = this.modal.querySelector('.theme-option-card[aria-checked="true"]') ||
                          this.modal.querySelector('.theme-option-card') ||
                          this.modal.querySelector('.modal-close');
      firstChoice?.focus();
    },

    closeModal() {
      if (!this.modal) return;
      this.modal.classList.remove('open');
      this.modal.setAttribute('aria-hidden', 'true');
      document.body.classList.remove('modal-open');

      if (this.previousFocus instanceof HTMLElement) {
        this.previousFocus.focus();
        this.previousFocus = null;
      }
    },

    bindEvents() {
      // System dark mode change listener
      this.systemDarkMedia.addEventListener?.('change', () => {
        if (this.state.theme === 'system') {
          this.applyTheme();
        }
      });

      // Trigger buttons to open appearance modal
      const openTriggers = [
        document.getElementById('topbarAppearanceTrigger'),
        document.getElementById('statusBarAppearanceTrigger'),
        document.getElementById('themeToggle'),
        ...document.querySelectorAll('[data-open-modal="appearanceModal"]')
      ].filter(Boolean);

      openTriggers.forEach((btn) => {
        btn.addEventListener('click', (e) => {
          e.preventDefault();
          this.openModal();
        });
      });

      if (!this.modal) return;

      // Close handlers
      this.modal.querySelector('#closeAppearanceModal')?.addEventListener('click', () => this.closeModal());
      this.modal.querySelector('#doneAppearanceModal')?.addEventListener('click', () => this.closeModal());
      this.modal.addEventListener('click', (e) => {
        if (e.target === this.modal) this.closeModal();
      });

      // Modal keyboard trap and Escape
      this.modal.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
          e.preventDefault();
          this.closeModal();
          return;
        }

        if (e.key === 'Tab') {
          const focusables = Array.from(
            this.modal.querySelectorAll('button:not(:disabled), input:not(:disabled), [tabindex="0"]')
          ).filter(el => el.offsetParent !== null);
          if (!focusables.length) return;

          const first = focusables[0];
          const last = focusables[focusables.length - 1];

          if (e.shiftKey && document.activeElement === first) {
            e.preventDefault();
            last.focus();
          } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault();
            first.focus();
          }
        }
      });

      // Theme choices
      this.modal.querySelectorAll('[data-theme-choice]').forEach((btn) => {
        btn.addEventListener('click', () => {
          this.state.theme = btn.dataset.themeChoice;
          this.savePreferences();
          this.applyTheme();
          this.syncControls();
        });
      });

      // Accent presets
      this.modal.querySelectorAll('[data-accent-choice]').forEach((btn) => {
        btn.addEventListener('click', () => {
          this.state.accent = btn.dataset.accentChoice;
          this.savePreferences();
          this.applyAccent();
          this.syncControls();
        });
      });

      // Custom color picker
      const customInput = this.modal.querySelector('#customAccentInput');
      const customValue = this.modal.querySelector('#customAccentValue');
      if (customInput) {
        customInput.addEventListener('input', (e) => {
          const hex = e.target.value;
          this.state.accent = 'custom';
          this.state.customColor = hex;
          if (customValue) customValue.textContent = hex.toUpperCase();
          this.applyAccent();
          this.syncControls();
        });
        customInput.addEventListener('change', () => {
          this.savePreferences();
        });
      }

      // Density choices
      this.modal.querySelectorAll('[data-density-choice]').forEach((btn) => {
        btn.addEventListener('click', () => {
          this.state.density = btn.dataset.densityChoice;
          this.savePreferences();
          this.applyDensity();
          this.syncControls();
        });
      });

      // Motion choices
      this.modal.querySelectorAll('[data-motion-choice]').forEach((btn) => {
        btn.addEventListener('click', () => {
          this.state.motion = btn.dataset.motionChoice;
          this.savePreferences();
          this.applyMotion();
          this.syncControls();
        });
      });
    }
  };

  // ---------------------------------------------------------------------------
  // 04. NETWORK & OFFLINE STATUS MANAGER
  // ---------------------------------------------------------------------------
  const NetworkStatusManager = {
    banner: null,
    topPresenceDot: null,
    statusBarDot: null,
    statusBarLabel: null,
    isOnline: true,

    init() {
      this.banner = document.getElementById('offlineBanner');
      this.topPresenceDot = document.getElementById('topPresenceDot');
      this.statusBarDot = document.getElementById('networkStatusDot');
      this.statusBarLabel = document.getElementById('networkStatusLabel');
      this.isOnline = navigator.onLine !== false;

      this.updateUI(this.isOnline, false);

      window.addEventListener('online', () => {
        this.isOnline = true;
        this.updateUI(true, true);
      });

      window.addEventListener('offline', () => {
        this.isOnline = false;
        this.updateUI(false, true);
      });
    },

    updateUI(online, notify = false) {
      if (this.banner) {
        this.banner.classList.toggle('visible', !online);
      }

      const presenceDots = [this.topPresenceDot, this.statusBarDot].filter(Boolean);
      presenceDots.forEach((dot) => {
        dot.classList.toggle('online', online);
        dot.classList.toggle('offline', !online);
        dot.title = online ? 'Система активна · В сети' : 'Оффлайн-режим';
      });

      if (this.statusBarLabel) {
        this.statusBarLabel.textContent = online ? 'В сети' : 'Не в сети';
      }

      if (notify) {
        if (online) {
          ToastManager.show('Подключение к сети восстановлено.', 'success', 2800);
        } else {
          ToastManager.show('Подключение потеряно. Изменения сохраняются локально.', 'warning', 4500);
        }
      }
    }
  };

  // ---------------------------------------------------------------------------
  // 05. CONTEXTUAL STATUS BAR MANAGER
  // ---------------------------------------------------------------------------
  const StatusBarManager = {
    bar: null,
    syncLabel: null,
    syncIcon: null,
    contextText: null,

    init() {
      this.bar = document.getElementById('appStatusBar');
      this.syncLabel = document.getElementById('syncStatusLabel');
      this.syncIcon = document.getElementById('syncStatusIcon');
      this.contextText = document.getElementById('contextualStatusText');

      this.setupReadingContext();
      this.setupEditorContext();
      this.setupAutosaveEvents();
    },

    setupReadingContext() {
      const readerPage = document.querySelector('.reader-page');
      if (!readerPage) return;

      const markdown = readerPage.querySelector('.doc-main .markdown');
      const text = markdown?.textContent || '';
      const words = text.trim() ? text.trim().split(/\s+/).length : 0;
      const readingMinutes = Math.max(1, Math.ceil(words / 190));
      const title = readerPage.querySelector('h1')?.textContent?.trim() || '';

      if (this.contextText && title) {
        this.contextText.textContent = `${title} · ${words} слов · ~${readingMinutes} мин чтения`;
      }

      // Reading progress bar
      const progressBar = document.getElementById('readingProgressBar');
      if (progressBar && markdown) {
        const updateProgress = () => {
          const rect = markdown.getBoundingClientRect();
          const totalHeight = rect.height - window.innerHeight;
          if (totalHeight <= 0) {
            progressBar.style.width = '100%';
            return;
          }
          const progress = Math.max(0, Math.min(100, ((-rect.top) / totalHeight) * 100));
          progressBar.style.width = `${progress}%`;
        };
        window.addEventListener('scroll', updateProgress, { passive: true });
        updateProgress();
      }
    },

    setupEditorContext() {
      const editor = document.getElementById('content');
      const wordCountBadge = document.getElementById('editorWordCount');
      if (!editor) return;

      const updateCount = () => {
        const text = editor.value.trim();
        const words = text ? text.split(/\s+/).length : 0;
        if (wordCountBadge) wordCountBadge.textContent = `${words} слов`;
        if (this.contextText) {
          const title = document.getElementById('articleTitle')?.value?.trim() || 'Новый документ';
          this.contextText.textContent = `Черновик · ${title} · ${words} слов`;
        }
      };

      editor.addEventListener('input', debounce(updateCount, 150));
      updateCount();
    },

    setupAutosaveEvents() {
      window.addEventListener('dh:autosave-state', (e) => {
        const { state, message } = e.detail || {};
        if (!this.syncLabel) return;

        if (state === 'saving') {
          this.syncLabel.textContent = 'Сохраняем…';
          if (this.syncIcon) {
            this.syncIcon.innerHTML = '<circle cx="12" cy="12" r="10" stroke-dasharray="32" stroke-dashoffset="12"/>';
            this.syncIcon.classList.add('icon-spin');
          }
        } else if (state === 'saved') {
          this.syncLabel.textContent = message || 'Сохранено';
          if (this.syncIcon) {
            this.syncIcon.innerHTML = '<path d="M20 6L9 17l-5-5"/>';
            this.syncIcon.classList.remove('icon-spin');
            this.syncIcon.classList.add('icon-check-animated');
          }
        } else if (state === 'dirty') {
          this.syncLabel.textContent = 'Есть правки';
          if (this.syncIcon) {
            this.syncIcon.innerHTML = '<circle cx="12" cy="12" r="3"/>';
            this.syncIcon.classList.remove('icon-spin');
          }
        } else if (state === 'error') {
          this.syncLabel.textContent = 'Сохранено локально';
          if (this.syncIcon) {
            this.syncIcon.innerHTML = '<path d="M12 9v2m0 4h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0z"/>';
            this.syncIcon.classList.remove('icon-spin');
          }
        }
      });
    }
  };

  // ---------------------------------------------------------------------------
  // 06. MARKDOWN & CODE ENHANCEMENTS
  // ---------------------------------------------------------------------------
  function enhanceMarkdown(root = document) {
    // Table wrap for responsive horizontal scroll
    root.querySelectorAll('.markdown table').forEach((table) => {
      if (table.closest('.table-wrap')) return;
      const wrapper = document.createElement('div');
      wrapper.className = 'table-wrap';
      table.before(wrapper);
      wrapper.appendChild(table);
    });

    // Copy to clipboard buttons on code blocks
    root.querySelectorAll('.markdown pre').forEach((pre) => {
      if (pre.querySelector('.code-copy')) return;
      const button = document.createElement('button');
      button.className = 'code-copy';
      button.type = 'button';
      button.setAttribute('aria-label', 'Копировать блок кода');
      button.innerHTML = `
        <svg class="icon icon-xs code-copy-icon" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
        <span class="code-copy-text">Копировать</span>
      `;

      button.addEventListener('click', async () => {
        const text = pre.querySelector('code')?.textContent || '';
        try {
          await navigator.clipboard.writeText(text);
          button.innerHTML = `
            <svg class="icon icon-xs icon-check-animated" viewBox="0 0 24 24"><path d="M20 6L9 17l-5-5"/></svg>
            <span class="code-copy-text">Скопировано</span>
          `;
          button.classList.add('is-copied');

          window.setTimeout(() => {
            button.innerHTML = `
              <svg class="icon icon-xs code-copy-icon" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
              <span class="code-copy-text">Копировать</span>
            `;
            button.classList.remove('is-copied');
          }, 1800);
        } catch (_) {
          ToastManager.show('Не удалось скопировать код в буфер', 'error');
        }
      });
      pre.appendChild(button);
    });
  }

  // ---------------------------------------------------------------------------
  // 07. MOBILE NAVIGATION & KEYBOARD VIEWPORT
  // ---------------------------------------------------------------------------
  const NavigationManager = {
    navToggle: null,
    navClose: null,
    sidepanel: null,
    backdrop: null,
    appMain: null,
    mobileMedia: window.matchMedia('(max-width: 900px)'),
    previousFocus: null,

    init() {
      this.navToggle = document.querySelector('.mobile-nav-toggle');
      this.navClose = document.querySelector('.mobile-nav-close');
      this.sidepanel = document.querySelector('.sidepanel');
      this.backdrop = document.querySelector('.mobile-backdrop');
      this.appMain = document.querySelector('.app-main');

      this.syncMode();
      this.mobileMedia.addEventListener?.('change', () => this.syncMode());

      this.navToggle?.addEventListener('click', () => {
        this.setOpen(!document.body.classList.contains('nav-open'));
      });
      this.navClose?.addEventListener('click', () => this.setOpen(false));
      this.backdrop?.addEventListener('click', () => this.setOpen(false));

      this.sidepanel?.addEventListener('click', (e) => {
        if (e.target.closest('a, button[type="submit"]')) {
          this.setOpen(false);
        }
      });

      document.addEventListener('keydown', (e) => {
        if (!document.body.classList.contains('nav-open')) return;
        if (e.key === 'Escape') {
          e.preventDefault();
          this.setOpen(false);
          return;
        }
        if (e.key === 'Tab' && this.sidepanel) {
          const focusables = Array.from(
            this.sidepanel.querySelectorAll('a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled)')
          ).filter(el => el.offsetParent !== null);
          if (!focusables.length) return;
          const first = focusables[0];
          const last = focusables[focusables.length - 1];

          if (e.shiftKey && document.activeElement === first) {
            e.preventDefault();
            last.focus();
          } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault();
            first.focus();
          }
        }
      });

      // Visual Viewport Sync for Mobile Virtual Keyboard
      const visualViewport = window.visualViewport;
      const syncKeyboardOffset = () => {
        const offset = visualViewport
          ? Math.max(0, window.innerHeight - visualViewport.height - visualViewport.offsetTop)
          : 0;
        document.documentElement.style.setProperty('--keyboard-offset', `${Math.round(offset)}px`);
      };
      visualViewport?.addEventListener('resize', syncKeyboardOffset);
      visualViewport?.addEventListener('scroll', syncKeyboardOffset);
      syncKeyboardOffset();
    },

    setOpen(open) {
      const isOpen = Boolean(open && this.mobileMedia.matches);
      document.body.classList.toggle('nav-open', isOpen);
      this.navToggle?.setAttribute('aria-expanded', String(isOpen));
      if (this.sidepanel) this.sidepanel.inert = this.mobileMedia.matches && !isOpen;
      if (this.appMain) this.appMain.inert = isOpen;

      if (isOpen) {
        this.previousFocus = document.activeElement;
        this.navClose?.focus();
      } else if (this.previousFocus instanceof HTMLElement) {
        const target = this.previousFocus;
        this.previousFocus = null;
        window.requestAnimationFrame(() => target.focus());
      }
    },

    syncMode() {
      if (!this.mobileMedia.matches) {
        this.previousFocus = null;
        document.body.classList.remove('nav-open');
        this.navToggle?.setAttribute('aria-expanded', 'false');
        if (this.sidepanel) this.sidepanel.inert = false;
        if (this.appMain) this.appMain.inert = false;
      } else if (!document.body.classList.contains('nav-open') && this.sidepanel) {
        this.sidepanel.inert = true;
        if (this.appMain) this.appMain.inert = false;
      }
    }
  };

  // ---------------------------------------------------------------------------
  // 08. EDITOR WORKSPACE & FILE UPLOADS
  // ---------------------------------------------------------------------------
  function setupEditor() {
    const editor = document.getElementById('content');
    const preview = document.getElementById('preview');
    if (!editor || !preview) return;

    const dropzone = document.getElementById('dropzone');
    const fileInput = document.getElementById('mediaInput');
    const picker = document.getElementById('mediaPicker');
    let previewController = null;

    const render = async () => {
      previewController?.abort();
      previewController = new AbortController();
      try {
        const response = await apiFetch('/api/preview', {
          method: 'POST',
          headers: { 'Content-Type': 'text/plain; charset=utf-8' },
          body: editor.value,
          signal: previewController.signal,
        });

        if (response.redirected && new URL(response.url).pathname === '/login') {
          window.location.assign('/login');
          return;
        }

        if (!response.ok) {
          throw new Error((await response.text()).trim() || 'Предпросмотр недоступен');
        }

        preview.innerHTML = await response.text();
        enhanceMarkdown(preview);

        if (window.mermaid) {
          const diagrams = preview.querySelectorAll('.mermaid:not([data-processed])');
          if (diagrams.length) {
            await window.mermaid.run({ nodes: diagrams });
          }
        }
      } catch (error) {
        if (error.name !== 'AbortError') {
          ToastManager.show(error.message || 'Не удалось обновить предпросмотр', 'error');
        }
      }
    };

    const scheduleRender = debounce(render, 220);
    const editorGrid = document.querySelector('.editorgrid');
    const editPane = document.getElementById('editorPane');
    const previewPane = document.getElementById('previewPane');
    const paneButtons = Array.from(document.querySelectorAll('[data-editor-pane]'));
    const mobileEditor = window.matchMedia('(max-width: 900px)');

    const setEditorPane = (pane, focusPane = false) => {
      const next = pane === 'preview' ? 'preview' : 'edit';
      if (editorGrid) editorGrid.dataset.mobilePane = next;
      paneButtons.forEach((btn) => {
        const selected = btn.dataset.editorPane === next;
        btn.setAttribute('aria-selected', String(selected));
        btn.tabIndex = selected ? 0 : -1;
      });
      const isMobile = mobileEditor.matches;
      if (editPane) editPane.hidden = isMobile && next !== 'edit';
      if (previewPane) previewPane.hidden = isMobile && next !== 'preview';
      if (focusPane) {
        (next === 'preview' ? previewPane : editPane)?.focus?.({ preventScroll: true });
      }
    };

    paneButtons.forEach((btn) => {
      btn.addEventListener('click', async () => {
        const pane = btn.dataset.editorPane;
        if (pane === 'preview') await render();
        setEditorPane(pane);
      });
    });

    mobileEditor.addEventListener?.('change', () => {
      setEditorPane(editorGrid?.dataset.mobilePane || 'edit');
    });
    setEditorPane(editorGrid?.dataset.mobilePane || 'edit');

    editor.addEventListener('input', scheduleRender);

    // Paste files
    editor.addEventListener('paste', async (e) => {
      const items = Array.from(e.clipboardData?.items || []);
      const fileItems = items.filter(item => item.kind === 'file').map(item => item.getAsFile()).filter(Boolean);
      if (fileItems.length) {
        e.preventDefault();
        await uploadFiles(fileItems, editor, render, dropzone);
      }
    });

    picker?.addEventListener('click', () => fileInput?.click());
    fileInput?.addEventListener('change', () => {
      uploadFiles(Array.from(fileInput.files || []), editor, render, dropzone).finally(() => {
        fileInput.value = '';
      });
    });

    // Drag and drop files
    [dropzone, editor].filter(Boolean).forEach((target) => {
      target.addEventListener('dragenter', (e) => {
        if (!hasFiles(e)) return;
        e.preventDefault();
        dropzone?.classList.add('is-dragging');
      });
      target.addEventListener('dragover', (e) => {
        if (!hasFiles(e)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'copy';
      });
      target.addEventListener('dragleave', (e) => {
        if (!dropzone?.contains(e.relatedTarget)) {
          dropzone?.classList.remove('is-dragging');
        }
      });
      target.addEventListener('drop', (e) => {
        if (!hasFiles(e)) return;
        e.preventDefault();
        dropzone?.classList.remove('is-dragging');
        uploadFiles(Array.from(e.dataTransfer.files || []), editor, render, dropzone);
      });
    });

    render();
  }

  function insertAtCursor(textarea, text) {
    const start = textarea.selectionStart ?? textarea.value.length;
    const end = textarea.selectionEnd ?? start;
    textarea.setRangeText(text, start, end, 'end');
  }

  function isSupportedFile(file) {
    if (/^(image|audio|video)\//.test(file.type || '') || file.type === 'application/pdf') return true;
    return /\.(avif|gif|jpe?g|png|webp|aac|flac|m4a|mp3|oga|ogg|wav|webm|mov|m4v|mp4|pdf)$/i.test(file.name || '');
  }

  function hasFiles(e) {
    return Array.from(e.dataTransfer?.types || []).includes('Files');
  }

  async function uploadFiles(files, targetEditor, render, dropzone) {
    const allowed = files.filter(isSupportedFile);
    if (!allowed.length) {
      ToastManager.show('Этот формат файла не поддерживается', 'warning');
      return;
    }
    const progress = dropzone?.querySelector('p');
    const progressDefault = progress?.textContent || '';
    dropzone?.classList.add('is-uploading');

    try {
      for (const [index, file] of allowed.entries()) {
        if (progress) progress.textContent = `Загрузка ${index + 1} из ${allowed.length}: ${file.name}`;
        const form = new FormData();
        form.append('file', file);
        const response = await apiFetch(targetEditor.dataset.uploadEndpoint || '/api/uploads', {
          method: 'POST',
          body: form
        });
        if (!response.ok) {
          throw new Error((await response.text()).trim() || `Не удалось загрузить ${file.name}`);
        }
        const payload = await response.json();
        insertAtCursor(targetEditor, `\n\n${payload.markdown}\n\n`);
      }
      targetEditor.dispatchEvent(new Event('input', { bubbles: true }));
      await render();
      targetEditor.focus();
      ToastManager.show(allowed.length === 1 ? 'Файл успешно добавлен' : `Добавлено файлов: ${allowed.length}`, 'success');
    } catch (error) {
      ToastManager.show(error.message || 'Не удалось загрузить вложение', 'error');
    } finally {
      dropzone?.classList.remove('is-uploading');
      if (progress) progress.textContent = progressDefault;
    }
  }

  // ---------------------------------------------------------------------------
  // 09. GLOBAL KEYBOARD SHORTCUTS
  // ---------------------------------------------------------------------------
  document.addEventListener('keydown', (e) => {
    // Ctrl/Cmd + S to save in editor
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
      const form = document.activeElement?.closest?.('form.editor') || document.querySelector('form.editor');
      if (form) {
        e.preventDefault();
        form.requestSubmit();
      }
    }

    // '/' to focus global search when not inside inputs
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const tag = document.activeElement?.tagName;
      if (!['INPUT', 'TEXTAREA', 'SELECT'].includes(tag) && !document.activeElement?.isContentEditable) {
        e.preventDefault();
        document.getElementById('globalSearch')?.focus();
      }
    }
  });

  // ---------------------------------------------------------------------------
  // 10. INITIALIZATION BOOTSTRAP
  // ---------------------------------------------------------------------------
  document.addEventListener('DOMContentLoaded', () => {
    AppearanceManager.init();
    NetworkStatusManager.init();
    StatusBarManager.init();
    NavigationManager.init();
    enhanceMarkdown();
    setupEditor();
  });
})();
