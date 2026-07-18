import { test, expect, Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const adminPassword = process.env.E2E_ADMIN_PASSWORD;
if (!adminPassword) throw new Error('E2E_ADMIN_PASSWORD is required for Playwright tests');

async function login(page: Page) {
  await page.goto('/login');
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

  if ((page.viewportSize()?.width || 1000) > 760) {
    await expect(page.getByRole('navigation', { name: 'Разделы' })).toBeVisible();
  }

  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();
  const blocking = accessibility.violations.filter((violation) => ['critical', 'serious'].includes(violation.impact || ''));
  expect(blocking, JSON.stringify(blocking, null, 2)).toEqual([]);

  if ((page.viewportSize()?.width || 1000) <= 760) {
    await page.getByRole('button', { name: 'Открыть навигацию' }).click();
    await expect(page.locator('body')).toHaveClass(/nav-open/);
    await expect(page.getByRole('button', { name: 'Закрыть навигацию' })).toBeFocused();
    await expect(page.getByRole('navigation', { name: 'Разделы' })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('body')).not.toHaveClass(/nav-open/);
  }
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

test('PDF загружается, связывается с документом и открывается в viewer', async ({ page }, testInfo) => {
  const title = `PDF ${Date.now()} ${testInfo.project.name}`;
  await page.goto('/new');
  await page.getByRole('textbox', { name: 'Заголовок документа' }).fill(title);
  await page.locator('#mediaInput').setInputFiles({
    name: 'ui-audit.pdf',
    mimeType: 'application/pdf',
    buffer: Buffer.from('%PDF-1.4\n1 0 obj<</Type /Page>>endobj\n%%EOF'),
  });
  await expect(page.getByLabel('Текст документа в Markdown')).toHaveValue(/\/pdf\/viewer\//, { timeout: 10_000 });
  await page.getByRole('button', { name: /Сохранить/ }).click();
  await page.getByRole('link', { name: /ui-audit\.pdf/ }).click();
  await expect(page).toHaveURL(/\/pdf\/viewer\//);
  await expect(page.getByTitle('PDF: ui-audit.pdf')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Скачать PDF' })).toBeVisible();
});
