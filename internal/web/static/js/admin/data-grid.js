document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('[data-filter-input]').forEach((input) => {
    const list = document.getElementById(input.dataset.filterInput);
    if (!list) return;
    const items = Array.from(list.querySelectorAll('[data-filter-text]'));
    const empty = document.createElement('p');
    empty.className = 'u-muted';
    empty.textContent = 'Совпадений нет.';
    empty.hidden = true;
    list.appendChild(empty);
    input.addEventListener('input', () => {
      const query = input.value.trim().toLocaleLowerCase('ru');
      let visible = 0;
      items.forEach((item) => {
        const matches = !query || item.textContent.toLocaleLowerCase('ru').includes(query);
        item.hidden = !matches;
        if (matches) visible += 1;
      });
      empty.hidden = visible !== 0;
    });
  });

  document.querySelectorAll('[data-confirm]').forEach((button) => {
    button.addEventListener('click', (event) => {
      if (!window.confirm(button.dataset.confirm || 'Подтвердить действие?')) event.preventDefault();
    });
  });
});
