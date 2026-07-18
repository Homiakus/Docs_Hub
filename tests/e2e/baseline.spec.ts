import { test, expect, Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const adminPassword = process.env.E2E_ADMIN_PASSWORD;
if (!adminPassword) throw new Error('E2E_ADMIN_PASSWORD is required for Playwright tests');
const mobileBreakpoint = 900;

function validPDF(text = 'Docs Hub mobile PDF audit'): Buffer {
  const safeText = text.replace(/([\\()])/g, '\\$1');
  const stream = `BT\n/F1 18 Tf\n72 720 Td\n(${safeText}) Tj\nET\n`;
  const objects = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    `<< /Length ${Buffer.byteLength(stream, 'latin1')} >>\nstream\n${stream}endstream`,
  ];
  let pdf = '%PDF-1.4\n%\xFF\xFF\xFF\xFF\n';
  const offsets = [0];
  objects.forEach((object, index) => {
    offsets.push(Buffer.byteLength(pdf, 'latin1'));
    pdf += `${index + 1} 0 obj\n${object}\nendobj\n`;
  });
  const xrefOffset = Buffer.byteLength(pdf, 'latin1');
  pdf += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`;
  pdf += offsets.slice(1).map((offset) => `${String(offset).padStart(10, '0')} 00000 n \n`).join('');
  pdf += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xrefOffset}\n%%EOF\n`;
  return Buffer.from(pdf, 'latin1');
}

async function expectNoHorizontalOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    content: document.documentElement.scrollWidth,
  }));
  expect(dimensions.content, JSON.stringify(dimensions)).toBeLessThanOrEqual(dimensions.viewport + 1);
}

async function expectTouchTargets(page: Page, selector: string) {
  const undersized = await page.locator(selector).evaluateAll((elements) => elements.flatMap((element) => {
    const node = element as HTMLElement;
    const style = window.getComputedStyle(node);
    const rect = node.getBoundingClientRect();
    if (style.display === 'none' || style.visibility === 'hidden' || !rect.width || !rect.height) return [];
    if (rect.width >= 43.5 && rect.height >= 43.5) return [];
    return [`${node.id || node.className || node.tagName}: ${Math.round(rect.width)}×${Math.round(rect.height)}`];
  }));
  expect(undersized, undersized.join('\n')).toEqual([]);
}

async function login(page: Page) {
  await page.goto('/login');
  await expectNoHorizontalOverflow(page);
  if ((page.viewportSize()?.width || 1000) <= mobileBreakpoint) {
    await expectTouchTargets(page, '.login-form .form-control, .login-form .btn');
  }
  await page.getByLabel('Логин').fill('admin');
  await page.getByLabel('Пароль').fill(adminPassword);
  await page.getByRole('button', { name: 'Войти' }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator('.app-shell')).toBeVisible();
}

async function createDocument(page: Page, title: string, content: string) {
  await page.goto('/new');
  await page.getByRole('textbox', { name: 'Заголовок документа' }).fill(title);
  await page.getByLabel('Текст документа в Markdown').fill(content);
  await page.getByRole('button', { name: /Сохранить/ }).click();
  await expect(page).toHaveURL(/\/a\//);
  await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible();
}

test.beforeEach(async ({ page }) => login(page));

test('основной shell доступен с клавиатуры и на мобильном экране', async ({ page }) => {
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Добро пожаловать');
  await expect(page.getByRole('search')).toBeVisible();

  const viewportWidth = page.viewportSize()?.width || 1000;
  if (viewportWidth > mobileBreakpoint) {
    await expect(page.getByRole('navigation', { name: 'Разделы' })).toBeVisible();
  }

  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze();
  const blocking = accessibility.violations.filter((violation) => ['critical', 'serious'].includes(violation.impact || ''));
  expect(blocking, JSON.stringify(blocking, null, 2)).toEqual([]);

  if (viewportWidth <= mobileBreakpoint) {
    const navToggle = page.getByRole('button', { name: 'Открыть навигацию' });
    await navToggle.click();
    await expect(page.locator('body')).toHaveClass(/nav-open/);
    await expect(page.getByRole('button', { name: 'Закрыть навигацию' })).toBeFocused();
    await expect(page.getByRole('navigation', { name: 'Разделы' })).toBeVisible();
    await expectTouchTargets(page, '.mobile-nav-close, .sidepanel .side-link');
    await page.keyboard.press('Escape');
    await expect(page.locator('body')).not.toHaveClass(/nav-open/);
    await expect(navToggle).toBeFocused();
    await expectTouchTargets(page, '.mobile-nav-toggle, .global-search, .top-actions .btn, .top-actions .icon-button');
  }
  await expectNoHorizontalOverflow(page);
});

test('мобильный редактор переключает панели и сохраняет крупные touch-цели', async ({ page }) => {
  test.skip((page.viewportSize()?.width || 1000) > mobileBreakpoint, 'Проверка относится к tablet/mobile layout');
  await page.goto('/new');
  const editorTab = page.getByRole('tab', { name: 'Редактор' });
  const previewTab = page.getByRole('tab', { name: 'Предпросмотр' });
  await expect(editorTab).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('#editorPane')).toBeVisible();
  await expect(page.locator('#previewPane')).toBeHidden();
  await page.getByLabel('Текст документа в Markdown').fill('## Мобильный аудит\n\nПредпросмотр работает без горизонтальной прокрутки.');
  await previewTab.click();
  await expect(previewTab).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('#editorPane')).toBeHidden();
  await expect(page.locator('#previewPane')).toBeVisible();
  await expect(page.locator('#preview')).toContainText('Мобильный аудит');

  const settings = page.locator('.editor-settings');
  await settings.getByRole('button', { name: 'Свойства документа' }).click();
  await expect(settings).not.toHaveAttribute('open', '');
  await settings.getByRole('button', { name: 'Свойства документа' }).click();
  await expect(settings).toHaveAttribute('open', '');
  await expectTouchTargets(page, '#editorTabEdit, #editorTabPreview, .toolbar-group button, #mediaPicker, .editor-mobile-actions .btn');
  await expectNoHorizontalOverflow(page);
});

test('редактор остаётся управляемым в мобильной landscape-ориентации', async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.startsWith('Mobile'), 'Проверка относится к мобильной landscape-ориентации');
  await page.setViewportSize({ width: 844, height: 390 });
  await page.goto('/new');
  await expect(page.getByRole('tab', { name: 'Редактор' })).toBeVisible();
  const actions = page.locator('.editor-mobile-actions');
  await expect(actions).toBeVisible();
  const bounds = await actions.boundingBox();
  expect(bounds).not.toBeNull();
  expect((bounds?.y || 0) + (bounds?.height || 0)).toBeLessThanOrEqual(391);
  await expectNoHorizontalOverflow(page);
});

test('command palette поддерживает поиск и стрелочную навигацию', async ({ page }) => {
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+K' : 'Control+K');
  const palette = page.getByRole('dialog', { name: 'Быстрый поиск и команды' });
  await expect(palette).toBeVisible();
  const combobox = page.getByRole('combobox', { name: 'Результаты' }).or(page.locator('#commandPaletteInput'));
  await combobox.fill('поиск');
  await page.waitForTimeout(250);
  await page.keyboard.press('ArrowDown');
  await expect(combobox).toHaveAttribute('aria-activedescendant', /command-option-/);
  await page.keyboard.press('Escape');
  await expect(palette).toBeHidden();
});

test('автосохранение принимает id сервера и восстанавливает документ', async ({ page }, testInfo) => {
  const title = `Autosave ${Date.now()} ${testInfo.project.name}`;
  await page.goto('/new');
  await page.getByRole('textbox', { name: 'Заголовок документа' }).fill(title);
  await page.getByLabel('Текст документа в Markdown').fill('## Проверка\n\nЧерновик сохраняется атомарно.');
  await expect(page.locator('#autosaveIndicator')).toContainText(/Сохранено|Все изменения сохранены/, { timeout: 10_000 });
  await expect(page).toHaveURL(/\/edit\//);
  await expect(page.locator('input[name="id"]')).not.toHaveValue('0');
  await page.getByLabel('Текст документа в Markdown').fill('## Проверка\n\nЧерновик сохраняется атомарно.\n\nВторая версия.');
  await expect(page.locator('input[name="lock_version"]')).toHaveValue('2', { timeout: 10_000 });
  await page.reload();
  await expect(page.getByRole('textbox', { name: 'Заголовок документа' })).toHaveValue(title);
  await expect(page.getByLabel('Текст документа в Markdown')).toHaveValue(/Черновик сохраняется атомарно/);
});

test('workflow и фильтры поиска образуют связный сценарий', async ({ page }, testInfo) => {
  const title = `Published ${Date.now()} ${testInfo.project.name}`;
  await createDocument(page, title, '# Проверяемый документ\n\nУникальный контент workflow.');
  await page.getByRole('button', { name: 'Отправить на проверку' }).click();
  await expect(page.getByText('На проверке', { exact: true }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Одобрить' }).click();
  await expect(page.getByText('Одобрен', { exact: true }).first()).toBeVisible();
  await page.getByRole('button', { name: 'Опубликовать' }).click();
  await expect(page.getByText('Опубликован', { exact: true }).first()).toBeVisible();

  await page.goto('/search?status=published');
  await expect(page.getByRole('link', { name: `Открыть документ: ${title}` })).toBeVisible();
  await expect(page.locator('select[name="status"]')).toHaveValue('published');
});

test('граф допускает прокрутку страницы и остаётся управляемым на touch-экране', async ({ page }) => {
  await page.goto('/graph');
  const graph = page.locator('#graph');
  await expect(graph).toHaveAttribute('aria-busy', 'false');
  await expect(graph).toBeVisible();
  await graph.focus();
  await page.keyboard.press('+');
  if ((page.viewportSize()?.width || 1000) <= mobileBreakpoint) {
    await expect(graph).toHaveCSS('touch-action', 'pan-y');
    await expectTouchTargets(page, '#graphSearch, #graphStatus, .graph-toolbar-actions button');
  }
  await expectNoHorizontalOverflow(page);
});

test('административная рабочая область не создаёт page-level overflow', async ({ page }) => {
  await page.goto('/admin');
  await expect(page.getByRole('heading', { level: 1, name: 'Панель управления' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Разделы управления' })).toBeVisible();
  if ((page.viewportSize()?.width || 1000) <= mobileBreakpoint) {
    await expectTouchTargets(page, '.admin-anchor-nav a, .admin-section .form-control, .admin-section .btn');
  }
  await expectNoHorizontalOverflow(page);
});

test('PDF загружается, связывается с документом и открывается в viewer', async ({ page }, testInfo) => {
  const title = `PDF ${Date.now()} ${testInfo.project.name}`;
  await page.goto('/new');
  await page.getByRole('textbox', { name: 'Заголовок документа' }).fill(title);
  await page.locator('#mediaInput').setInputFiles({
    name: 'ui-audit.pdf',
    mimeType: 'application/pdf',
    buffer: validPDF(),
  });
  await expect(page.getByLabel('Текст документа в Markdown')).toHaveValue(/\/pdf\/viewer\//, { timeout: 10_000 });
  await page.getByRole('button', { name: /Сохранить/ }).click();
  await page.getByRole('link', { name: /ui-audit\.pdf/ }).click();
  await expect(page).toHaveURL(/\/pdf\/viewer\//);
  await expect(page.getByRole('heading', { level: 1, name: 'ui-audit.pdf' })).toBeVisible();
  await expect(page.locator('#pdfViewerContainer')).toHaveAttribute('aria-busy', 'false', { timeout: 20_000 });
  await expect(page.locator('#pdfCanvas')).toBeVisible();
  await expect(page.locator('iframe')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Скачать PDF' })).toBeVisible();
  await page.getByPlaceholder('Найти в PDF').fill('Docs Hub');
  await page.getByRole('button', { name: 'Найти текст' }).click();
  await expect(page.locator('#pdfSearchStatus')).toContainText('1 стр.');
  await expectNoHorizontalOverflow(page);
});
