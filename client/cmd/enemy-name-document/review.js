const rows = Array.from(document.querySelectorAll('tbody tr'));
const search = document.querySelector('#search');
const only = document.querySelector('#candidates');
const count = document.querySelector('#count');
const status = document.querySelector('#save-status');
const reviewCount = document.querySelector('#review-count');
const storageKey = 'gonline.entity-name-review.v1';
let edits = Object.create(null);
let storageAvailable = true;
try {
    const saved = JSON.parse(localStorage.getItem(storageKey) || '{}');
    if (saved && typeof saved === 'object' && !Array.isArray(saved)) edits = saved;
} catch (_) {
    storageAvailable = false;
}

const records = rows.map(row => {
    const input = row.querySelector('.name-edit');
    const certain = row.querySelector('.certain');
    const saved = edits[row.dataset.id];
    if (saved && saved.current_name === row.dataset.name && typeof saved.proposed_name === 'string') {
        input.value = saved.proposed_name;
        certain.checked = saved.certain === true && input.value.trim() !== '';
    }
    certain.disabled = input.value.trim() === '';
    return {row, input, certain, searchable: row.textContent.toLowerCase()};
});

function showProgress() {
    let named = 0, confident = 0;
    records.forEach(({input, certain}) => {
        if (input.value.trim()) {
            named++;
            if (certain.checked) confident++;
        }
    });
    reviewCount.textContent = `${named.toLocaleString()} names entered · ${confident.toLocaleString()} certain`;
    status.textContent = storageAvailable
        ? 'Edits are saved in this browser. Export a CSV to back them up or send them for correction.'
        : 'Browser storage is unavailable. Export your CSV before closing this document.';
}

function filter() {
    const value = search.value.trim().toLowerCase();
    let visible = 0;
    records.forEach(({row, input, searchable}) => {
        row.hidden = (!(searchable + ' ' + input.value.toLowerCase()).includes(value)
            || (only.checked && row.dataset.candidates !== 'yes'));
        if (!row.hidden) visible++;
    });
    count.textContent = `${visible.toLocaleString()} of ${rows.length.toLocaleString()} entities`;
}

records.forEach(record => {
    const {row, input, certain} = record;
    function save() {
        certain.disabled = input.value.trim() === '';
        if (certain.disabled) certain.checked = false;
        if (input.value.trim()) {
            edits[row.dataset.id] = {current_name: row.dataset.name,
                proposed_name: input.value, certain: certain.checked};
        } else {
            delete edits[row.dataset.id];
        }
        try {
            localStorage.setItem(storageKey, JSON.stringify(edits));
            storageAvailable = true;
        } catch (_) {
            storageAvailable = false;
        }
        showProgress();
        filter();
    }
    input.addEventListener('input', save);
    certain.addEventListener('change', save);
});

function csvCell(value) {
    return '"' + String(value).replace(/"/g, '""') + '"';
}

function exportCSV() {
    const lines = [['id', 'current_name', 'proposed_name', 'certain',
        'name_candidates', 'metadata_matches', 'sprite_id', 'review_notes']];
    // Always export the full inventory; filters affect only what is displayed.
    records.forEach(({row, input, certain}) => {
        const d = row.dataset;
        const proposed = input.value.trim();
        lines.push([d.id, d.name, proposed, proposed && certain.checked ? 'true' : 'false',
            d.leads, d.metadata, d.sprite, d.notes]);
    });
    return '\uFEFF' + lines.map(line => line.map(csvCell).join(',')).join('\r\n') + '\r\n';
}

document.querySelector('#export').addEventListener('click', () => {
    const blob = new Blob([exportCSV()], {type: 'text/csv;charset=utf-8'});
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'entity-name-corrections.csv';
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
    status.textContent = `Exported all ${rows.length.toLocaleString()} entities. Send entity-name-corrections.csv for name corrections.`;
});
search.addEventListener('input', filter);
only.addEventListener('change', filter);
showProgress();
filter();
