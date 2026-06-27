/*
 * Detail Panel — payee/category select sync + dynamic category suggestions
 *
 * Manual test checklist:
 *   [ ] Payee dropdown shows all payees; suggested payee is pre-selected
 *   [ ] Category dropdown shows all categories; suggested category is pre-selected
 *   [ ] Selecting a payee fetches category suggestions and narrows the category dropdown
 *   [ ] If only one category suggestion, it is auto-selected
 *   [ ] Clearing the payee restores the full category list and clears category selection
 *   [ ] Form submission sends payee ID and category ID (not display names)
 *   [ ] Clicking a second transaction re-initializes the panel correctly (JS rebound after HTMX swap)
 */

(function () {
    'use strict';

    // Snapshot of all categories as rendered by server — restored when payee is cleared/changed.
    var fullCategoryOptions = [];

    // Guards against stale fetch responses when payee changes rapidly.
    var latestPayeeId = '';

    function setCategoryOptions(options) {
        var sel = document.getElementById('detail-category-select');
        if (!sel) return;
        var current = sel.value;
        sel.innerHTML = '<option value="">— Select category —</option>';
        options.forEach(function (opt) {
            var el = document.createElement('option');
            el.value = opt.value;
            el.textContent = opt.text;
            if (opt.value && opt.value === current) el.selected = true;
            sel.appendChild(el);
        });
    }

    function restoreFullCategoryList() {
        setCategoryOptions(fullCategoryOptions);
    }

    function fetchCategorySuggestions(payeeId) {
        var form = document.getElementById('detail-form');
        if (!form) return;
        var budgetId = (form.querySelector('input[name="budget"]') || {}).value || '';
        var description = form.dataset.description || '';
        if (!budgetId || !payeeId) return;

        latestPayeeId = payeeId;

        var url = '/api/category-suggestions?budget_id=' + encodeURIComponent(budgetId) +
            '&description=' + encodeURIComponent(description) +
            '&payee_id=' + encodeURIComponent(payeeId);

        fetch(url)
            .then(function (r) { return r.json(); })
            .then(function (data) {
                if (latestPayeeId !== payeeId) return; // stale response
                var suggestions = (data && data.suggestions) || [];
                if (suggestions.length === 0) {
                    restoreFullCategoryList();
                    return;
                }
                var opts = suggestions.map(function (s) {
                    return { value: s.category_id, text: s.category_name };
                });
                setCategoryOptions(opts);
                // Auto-select if exactly one suggestion.
                if (suggestions.length === 1) {
                    var catSel = document.getElementById('detail-category-select');
                    if (catSel) catSel.value = suggestions[0].category_id;
                }
            })
            .catch(function () {
                restoreFullCategoryList();
            });
    }

    function updateRememberToggleState() {
        var row = document.querySelector('.remember-toggle-row');
        if (!row) return;
        var payeeSel = document.getElementById('detail-payee-select');
        var catSel = document.getElementById('detail-category-select');
        var ready = payeeSel && payeeSel.value && catSel && catSel.value;
        row.classList.toggle('disabled', !ready);
    }

    function onPayeeChange() {
        var payeeSel = document.getElementById('detail-payee-select');
        var payeeId = payeeSel ? payeeSel.value : '';

        if (payeeId) {
            fetchCategorySuggestions(payeeId);
        } else {
            latestPayeeId = '';
            restoreFullCategoryList();
            var catSel = document.getElementById('detail-category-select');
            if (catSel) catSel.value = '';
        }
        updateRememberToggleState();
    }

    function onCategoryChange() {
        updateRememberToggleState();
    }

    function init() {
        // Snapshot the full category list as rendered by the server (excluding blank placeholder).
        var catSel = document.getElementById('detail-category-select');
        if (catSel) {
            fullCategoryOptions = Array.from(catSel.querySelectorAll('option'))
                .filter(function (opt) { return opt.value !== ''; })
                .map(function (opt) {
                    return { value: opt.value, text: opt.textContent };
                });
        } else {
            fullCategoryOptions = [];
        }
        latestPayeeId = '';

        var payeeSel = document.getElementById('detail-payee-select');
        if (payeeSel) {
            payeeSel.removeEventListener('change', onPayeeChange);
            payeeSel.addEventListener('change', onPayeeChange);
        }

        var catSel = document.getElementById('detail-category-select');
        if (catSel) {
            catSel.removeEventListener('change', onCategoryChange);
            catSel.addEventListener('change', onCategoryChange);
        }

        updateRememberToggleState();
    }

    // Block the remember toggle from firing when no payee is selected.
    document.body.addEventListener('htmx:beforeRequest', function (e) {
        if (!e.target || !e.target.classList.contains('remember-checkbox')) return;
        var payeeSel = document.getElementById('detail-payee-select');
        var catSel = document.getElementById('detail-category-select');
        if (!payeeSel || !payeeSel.value || !catSel || !catSel.value) {
            e.preventDefault();
            e.target.checked = false;
        }
    });

    // Registered once outside init() so repeated HTMX panel swaps don't stack listeners.
    document.body.addEventListener('htmx:afterSettle', function (e) {
        if (e.detail && e.detail.target && e.detail.target.id === 'txn-detail-panel') {
            init();
        }
    });

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
