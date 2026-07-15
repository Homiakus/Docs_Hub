/* Admin Data Grid Helper Module */

document.addEventListener('DOMContentLoaded', () => {
  const grids = document.querySelectorAll('.data-grid-table');

  grids.forEach((table) => {
    const searchInput = table.parentElement.querySelector('.data-grid-search');
    if (!searchInput) return;

    searchInput.addEventListener('input', () => {
      const query = searchInput.value.toLowerCase().trim();
      const rows = table.querySelectorAll('tbody tr');

      rows.forEach((row) => {
        const text = row.textContent.toLowerCase();
        if (text.includes(query)) {
          row.style.display = '';
        } else {
          row.style.display = 'none';
        }
      });
    });
  });
});
