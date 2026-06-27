/*
 * Detail Panel — autocomplete ID sync + dynamic category suggestions
 *
 * Manual test checklist:
 *   [ ] Typing partial payee name filters the datalist options
 *   [ ] Selecting a payee sets hidden #detail-payee-id to correct UUID (not name)
 *   [ ] After payee select, category datalist updates with suggestions for that payee
 *   [ ] Selecting a category NOT in the payee suggestions still works (type its name, correct ID submitted)
 *   [ ] Clearing the payee restores the full category list and clears category input
 *   [ ] Selecting a category sets hidden #detail-category-id to correct UUID (not name)
 *   [ ] Form submission (Accept & Send to YNAB) sends payee ID and category ID, not display names
 *   [ ] Clicking a second transaction re-initializes the panel correctly (JS rebound after HTMX swap)
 */

(function () {
    'use strict';

    // Snapshot of all categories as rendered by server — restored when payee is cleared/changed.
    var fullCategoryOptions = [];

    // Guards against stale fetch responses when payee changes rapidly.
    var latestPayeeId = '';

    function findOption(datalist, value) {
        if (!datalist || !value) return null;
        var lower = value.toLowerCase();
        var options = datalist.querySelectorAll('option');
        for (var i = 0; i < options.length; i++) {
            if (options[i].value.toLowerCase() === lower) return options[i];
        }
        return null;
    }

    // Also searches fullCategoryOptions so a narrowed datalist doesn't lose the ID.
    function findCategoryOption(datalist, value) {
        var opt = findOption(datalist, value);
        if (opt) return opt;
        if (!value) return null;
        var lower = value.toLowerCase();
        for (var i = 0; i < fullCategoryOptions.length; i++) {
            if (fullCategoryOptions[i].value.toLowerCase() === lower) return fullCategoryOptions[i];
        }
        return null;
    }

    function setCategoryList(options) {
        var dl = document.getElementById('detail-categories-list');
        if (!dl) return;
        dl.innerHTML = '';
        options.forEach(function (opt) {
            var el = document.createElement('option');
            el.value = opt.value;
            el.dataset.id = opt.dataset.id;
            dl.appendChild(el);
        });
    }

    function restoreFullCategoryList() {
        setCategoryList(fullCategoryOptions);
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
                // Build synthetic option objects matching fullCategoryOptions shape.
                var opts = suggestions.map(function (s) {
                    var el = document.createElement('option');
                    el.value = s.category_name;
                    el.dataset.id = s.category_id;
                    return el;
                });
                setCategoryList(opts);
                // Auto-fill if exactly one suggestion.
                if (suggestions.length === 1) {
                    var catInput = document.getElementById('detail-category-input');
                    var catHidden = document.getElementById('detail-category-id');
                    if (catInput) catInput.value = suggestions[0].category_name;
                    if (catHidden) catHidden.value = suggestions[0].category_id;
                }
            })
            .catch(function () {
                restoreFullCategoryList();
            });
    }

    function syncPayee() {
        var input = document.getElementById('detail-payee-input');
        var hidden = document.getElementById('detail-payee-id');
        if (!input || !hidden) return;

        var dl = document.getElementById('detail-payees-list');
        var opt = findOption(dl, input.value.trim());

        if (opt && opt.dataset.id) {
            hidden.value = opt.dataset.id;
            fetchCategorySuggestions(opt.dataset.id);
        } else {
            hidden.value = '';
            latestPayeeId = '';
            restoreFullCategoryList();
            // Also clear category when payee is cleared.
            if (!input.value.trim()) {
                var catInput = document.getElementById('detail-category-input');
                var catHidden = document.getElementById('detail-category-id');
                if (catInput) catInput.value = '';
                if (catHidden) catHidden.value = '';
            }
        }
    }

    function syncCategory() {
        var input = document.getElementById('detail-category-input');
        var hidden = document.getElementById('detail-category-id');
        if (!input || !hidden) return;

        var dl = document.getElementById('detail-categories-list');
        var opt = findCategoryOption(dl, input.value.trim());

        hidden.value = (opt && opt.dataset.id) ? opt.dataset.id : '';
    }

    function init() {
        // Snapshot the full category list as rendered by the server.
        var dl = document.getElementById('detail-categories-list');
        if (dl) {
            fullCategoryOptions = Array.from(dl.querySelectorAll('option'));
        } else {
            fullCategoryOptions = [];
        }
        latestPayeeId = '';

        var payeeInput = document.getElementById('detail-payee-input');
        if (payeeInput) {
            payeeInput.removeEventListener('change', syncPayee);
            payeeInput.removeEventListener('blur', syncPayee);
            payeeInput.addEventListener('change', syncPayee);
            payeeInput.addEventListener('blur', syncPayee);
        }

        var catInput = document.getElementById('detail-category-input');
        if (catInput) {
            catInput.removeEventListener('change', syncCategory);
            catInput.removeEventListener('blur', syncCategory);
            catInput.addEventListener('change', syncCategory);
            catInput.addEventListener('blur', syncCategory);
        }
    }

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
