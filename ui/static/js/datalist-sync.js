/**
 * Datalist Sync - ID Synchronization for HTML5 Datalists
 * Syncs selected option text with corresponding data-id attribute to hidden fields
 */

(function() {
    'use strict';

    /**
     * Find the option in a datalist that matches the input value
     * @param {HTMLDataListElement} datalist
     * @param {string} value - The value from the input
     * @returns {HTMLOptionElement|null}
     */
    function findDatalistOption(datalist, value) {
        if (!datalist || !value) return null;

        const options = datalist.querySelectorAll('option');
        for (let option of options) {
            if (option.value.toLowerCase() === value.toLowerCase()) {
                return option;
            }
        }
        return null;
    }

    /**
     * Sync the ID from datalist option to hidden field
     * @param {HTMLInputElement} input - The text input with datalist
     */
    function syncDatalistId(input) {
        const listId = input.getAttribute('list');
        if (!listId) return;

        const datalist = document.getElementById(listId);
        if (!datalist) return;

        const value = input.value.trim();
        const option = findDatalistOption(datalist, value);

        // Find the corresponding hidden field
        let hiddenField = null;
        const row = input.closest('tr');

        if (input.name === 'payee') {
            hiddenField = row?.querySelector('.payee-id-field, input[name="payee_id"]');
        } else if (input.name === 'category') {
            hiddenField = row?.querySelector('.category-id-field, input[name="category_id"]');
        }

        if (hiddenField) {
            if (option && option.dataset.id) {
                hiddenField.value = option.dataset.id;
                input.dataset.payeeId = option.dataset.id; // Store on input for reference
            } else {
                // No match found - clear the ID
                hiddenField.value = '';
                input.dataset.payeeId = '';
            }
        }
    }

    /**
     * Handle input change event
     * @param {Event} event
     */
    function handleInputChange(event) {
        const input = event.target;

        // Only process inputs with datalists
        if (!input.hasAttribute('list')) return;

        syncDatalistId(input);
    }

    /**
     * Initialize datalist sync for all inputs
     */
    function init() {
        // Find all inputs with datalists
        const inputsWithDatalist = document.querySelectorAll('input[list]');

        inputsWithDatalist.forEach(input => {
            // Remove old listeners to avoid duplicates
            input.removeEventListener('change', handleInputChange);
            input.removeEventListener('blur', handleInputChange);

            // Add listeners
            input.addEventListener('change', handleInputChange);
            input.addEventListener('blur', handleInputChange);

            // Sync on init if there's already a value
            if (input.value) {
                syncDatalistId(input);
            }
        });
    }

    /**
     * Reinitialize after DOM changes (HTMX swaps)
     */
    function reinit() {
        init();
    }

    // Public API
    window.DatalistSync = {
        init: init,
        reinit: reinit,
        sync: syncDatalistId
    };

    // Initialize on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function reinitOnSwap(event) {
        // htmx:after:swap detail is {ctx}; the swapped target lives at detail.ctx.target.
        var target = event.detail.ctx?.target;
        // Only reinit if the swap might have added new datalist inputs
        if (target?.classList?.contains('txn-row') ||
            target?.id === 'transactions') {
            setTimeout(reinit, 10); // Small delay to ensure DOM is updated
        }
    }

    function reinitOnSettle(event) {
        // htmx:after:settle detail is {task, newContent, settleTasks}; the swapped
        // target lives at detail.task.target.
        var target = event.detail.task?.target;
        if (target?.classList?.contains('txn-row') ||
            target?.id === 'transactions') {
            setTimeout(reinit, 10);
        }
    }

    // htmx:after:swap/htmx:after:settle are the v4 event names (were
    // htmx:afterSwap/htmx:afterSettle in 1.x).
    document.body.addEventListener('htmx:after:swap', reinitOnSwap);
    document.body.addEventListener('htmx:after:settle', reinitOnSettle);

})();
