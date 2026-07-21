(function () {
    'use strict';

    // ─── SearchableSelect ────────────────────────────────────────────────────
    // Wraps a native <select> with a search-in-dropdown UI.
    // The native <select> stays hidden but drives form submission.

    function SearchableSelect(nativeSelect, placeholder) {
        this.select = nativeSelect;
        this.placeholder = placeholder || 'Search…';
        this._isOpen = false;
        this._build();
    }

    SearchableSelect.prototype._opts = function () {
        return Array.from(this.select.options)
            .filter(function (o) { return o.value !== ''; })
            .map(function (o) { return { value: o.value, text: o.textContent.trim() }; });
    };

    SearchableSelect.prototype._selectedText = function () {
        var sel = this.select;
        if (!sel.value) return '';
        var opt = Array.from(sel.options).find(function (o) { return o.value === sel.value; });
        return opt ? opt.textContent.trim() : '';
    };

    SearchableSelect.prototype._build = function () {
        var self = this;

        this.select.style.display = 'none';

        var w = document.createElement('div');
        w.className = 'ss-wrapper';
        this.select.parentNode.insertBefore(w, this.select);
        w.appendChild(this.select);
        this.wrapper = w;

        var trig = document.createElement('button');
        trig.type = 'button';
        trig.className = 'ss-trigger';
        this._updateTriggerText(trig);
        w.appendChild(trig);
        this.trigger = trig;

        var panel = document.createElement('div');
        panel.className = 'ss-panel';
        panel.style.display = 'none';
        w.appendChild(panel);
        this.panel = panel;

        var inp = document.createElement('input');
        inp.type = 'text';
        inp.className = 'ss-search';
        inp.placeholder = 'Search…';
        inp.autocomplete = 'off';
        panel.appendChild(inp);
        this.search = inp;

        var list = document.createElement('ul');
        list.className = 'ss-list';
        panel.appendChild(list);
        this.list = list;

        this._renderList('');

        trig.addEventListener('click', function (e) {
            e.stopPropagation();
            self._isOpen ? self.close() : self.open();
        });

        inp.addEventListener('input', function () {
            self._renderList(inp.value);
        });

        inp.addEventListener('keydown', function (e) {
            if (e.key === 'Escape') { self.close(); self.trigger.focus(); }
            if (e.key === 'ArrowDown') { self._focusListItem(0); e.preventDefault(); }
        });

        list.addEventListener('keydown', function (e) {
            var items = Array.from(list.querySelectorAll('.ss-option'));
            var idx = items.indexOf(document.activeElement);
            if (e.key === 'ArrowDown') { if (idx < items.length - 1) items[idx + 1].focus(); e.preventDefault(); }
            if (e.key === 'ArrowUp') {
                if (idx > 0) { items[idx - 1].focus(); }
                else { self.search.focus(); }
                e.preventDefault();
            }
            if (e.key === 'Enter' && idx >= 0) { items[idx].click(); e.preventDefault(); }
            if (e.key === 'Escape') { self.close(); self.trigger.focus(); }
        });

        document.addEventListener('click', function (e) {
            if (self._isOpen && !w.contains(e.target)) { self.close(); }
        });
    };

    SearchableSelect.prototype._focusListItem = function (idx) {
        var items = this.list.querySelectorAll('.ss-option');
        if (items[idx]) items[idx].focus();
    };

    SearchableSelect.prototype._updateTriggerText = function (trig) {
        var text = this._selectedText();
        (trig || this.trigger).textContent = text || '— ' + this.placeholder + ' —';
    };

    SearchableSelect.prototype._renderList = function (query) {
        var self = this;
        var q = query.toLowerCase();
        var opts = this._opts().filter(function (o) {
            return !q || o.text.toLowerCase().indexOf(q) !== -1;
        });

        this.list.innerHTML = '';

        if (opts.length === 0) {
            var empty = document.createElement('li');
            empty.className = 'ss-empty';
            empty.textContent = 'No results';
            this.list.appendChild(empty);
            return;
        }

        opts.forEach(function (o) {
            var li = document.createElement('li');
            li.className = 'ss-option' + (o.value === self.select.value ? ' ss-selected' : '');
            li.textContent = o.text;
            li.tabIndex = -1;
            li.dataset.value = o.value;
            li.addEventListener('mousedown', function (e) {
                e.preventDefault(); // keep search input focused until we explicitly close
                self._pick(o.value, o.text);
            });
            self.list.appendChild(li);
        });
    };

    SearchableSelect.prototype._pick = function (value, text) {
        this.select.value = value;
        this.trigger.textContent = text || '— ' + this.placeholder + ' —';
        this.close();
        this.select.dispatchEvent(new Event('change', { bubbles: true }));
    };

    SearchableSelect.prototype.open = function () {
        // Close sibling instances before opening this one.
        [ssPayee, ssCategory].forEach(function (ss) { if (ss && ss !== this && ss._isOpen) ss.close(); }, this);
        this.panel.style.display = 'flex';
        this.search.value = '';
        this._renderList('');
        this.search.focus();
        this._isOpen = true;
    };

    SearchableSelect.prototype.close = function () {
        this.panel.style.display = 'none';
        this._isOpen = false;
    };

    // Called after the underlying <select> options change externally.
    SearchableSelect.prototype.refresh = function () {
        this._updateTriggerText();
        if (this._isOpen) { this._renderList(this.search.value); }
    };

    // Programmatically set value (e.g. auto-select single category suggestion).
    SearchableSelect.prototype.setValue = function (value) {
        this.select.value = value;
        this._updateTriggerText();
    };

    SearchableSelect.prototype.destroy = function () {
        if (this.wrapper && this.wrapper.parentNode) {
            this.wrapper.parentNode.insertBefore(this.select, this.wrapper);
            this.select.style.display = '';
            this.wrapper.parentNode.removeChild(this.wrapper);
        }
        this.wrapper = null;
    };

    // ─── Detail Panel Logic ──────────────────────────────────────────────────

    var ssPayee = null;
    var ssCategory = null;
    var fullCategoryOptions = [];
    var latestPayeeId = '';

    function setCategoryOptions(options) {
        var sel = document.getElementById('detail-category-select');
        if (!sel) return;
        sel.innerHTML = '<option value=""></option>';
        options.forEach(function (opt) {
            var el = document.createElement('option');
            el.value = opt.value;
            el.textContent = opt.text;
            sel.appendChild(el);
        });
        if (ssCategory) ssCategory.refresh();
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
                if (latestPayeeId !== payeeId) return;
                var suggestions = (data && data.suggestions) || [];
                if (suggestions.length === 0) { restoreFullCategoryList(); return; }
                var opts = suggestions.map(function (s) {
                    return { value: s.category_id, text: s.category_name };
                });
                setCategoryOptions(opts);
                if (suggestions.length === 1 && ssCategory) {
                    ssCategory.setValue(suggestions[0].category_id);
                }
            })
            .catch(function () { restoreFullCategoryList(); });
    }

    function updateRememberToggleState() {
        var row = document.querySelector('.remember-toggle-row');
        if (!row) return;
        var payeeSel = document.getElementById('detail-payee-select');
        var catSel = document.getElementById('detail-category-select');
        row.classList.toggle('disabled', !(payeeSel && payeeSel.value && catSel && catSel.value));
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
            if (ssCategory) ssCategory.refresh();
        }
        updateRememberToggleState();
    }

    function init() {
        if (ssPayee) { ssPayee.destroy(); ssPayee = null; }
        if (ssCategory) { ssCategory.destroy(); ssCategory = null; }

        var payeeSel = document.getElementById('detail-payee-select');
        var catSel = document.getElementById('detail-category-select');

        if (catSel) {
            fullCategoryOptions = Array.from(catSel.options)
                .filter(function (o) { return o.value !== ''; })
                .map(function (o) { return { value: o.value, text: o.textContent.trim() }; });
        } else {
            fullCategoryOptions = [];
        }
        latestPayeeId = '';

        if (payeeSel) {
            ssPayee = new SearchableSelect(payeeSel, 'Select payee');
            payeeSel.addEventListener('change', onPayeeChange);
        }

        if (catSel) {
            ssCategory = new SearchableSelect(catSel, 'Select category');
            catSel.addEventListener('change', updateRememberToggleState);
        }

        updateRememberToggleState();
    }

    // Block remember toggle when no payee/category is set.
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
