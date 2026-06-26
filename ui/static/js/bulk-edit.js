/**
 * Bulk Edit - Checkbox Selection Management
 * Handles checkbox selection, bulk actions toolbar, and selection state
 */

(function() {
    'use strict';

    // State
    let selectedTxnIds = new Set();

    // DOM Elements
    function getElements() {
        return {
            selectAllCheckbox: document.getElementById('select-all'),
            txnCheckboxes: document.querySelectorAll('.txn-checkbox'),
            bulkToolbar: document.querySelector('.bulk-actions-toolbar'),
            selectionCount: document.querySelector('.selection-count'),
            bulkActionButtons: document.querySelectorAll('.bulk-action-btn')
        };
    }

    // Update selection count and toolbar visibility
    function updateSelectionUI() {
        const elements = getElements();
        const count = selectedTxnIds.size;

        if (elements.selectionCount) {
            elements.selectionCount.textContent = count === 1
                ? '1 transaction selected'
                : `${count} transactions selected`;
        }

        // Show/hide bulk toolbar
        if (elements.bulkToolbar) {
            elements.bulkToolbar.style.display = count > 0 ? 'flex' : 'none';
        }

        // Enable/disable bulk action buttons
        elements.bulkActionButtons.forEach(btn => {
            btn.disabled = count === 0;
        });

        // Update select-all checkbox state
        if (elements.selectAllCheckbox && elements.txnCheckboxes.length > 0) {
            const allChecked = elements.txnCheckboxes.length === count;
            const someChecked = count > 0 && count < elements.txnCheckboxes.length;

            elements.selectAllCheckbox.checked = allChecked;
            elements.selectAllCheckbox.indeterminate = someChecked;
        }
    }

    // Handle individual checkbox change
    function handleCheckboxChange(event) {
        const checkbox = event.target;
        const txnId = checkbox.dataset.txnId;

        if (!txnId) return;

        if (checkbox.checked) {
            selectedTxnIds.add(txnId);
        } else {
            selectedTxnIds.delete(txnId);
        }

        updateSelectionUI();
    }

    // Handle select-all checkbox
    function handleSelectAll(event) {
        const selectAll = event.target;
        const elements = getElements();

        elements.txnCheckboxes.forEach(checkbox => {
            // Skip disabled checkboxes (rows in edit mode)
            if (checkbox.disabled) return;

            checkbox.checked = selectAll.checked;
            const txnId = checkbox.dataset.txnId;

            if (selectAll.checked) {
                selectedTxnIds.add(txnId);
            } else {
                selectedTxnIds.delete(txnId);
            }
        });

        updateSelectionUI();
    }

    // Handle bulk skip button
    function handleBulkSkip(event) {
        event.preventDefault();

        const txnIds = Array.from(selectedTxnIds);
        if (txnIds.length === 0) {
            return;
        }

        // Confirm action
        const count = txnIds.length;
        const message = count === 1
            ? 'Skip 1 selected transaction?'
            : `Skip ${count} selected transactions?`;

        if (!confirm(message)) {
            return;
        }

        // Get account ID from table data attribute
        const table = document.querySelector('.transactions-table');
        const accountId = table?.dataset?.accountId || '';

        if (!accountId) {
            alert('Could not determine account ID. Please refresh and try again.');
            return;
        }

        // Build URL-encoded form data
        const params = new URLSearchParams();
        txnIds.forEach(id => {
            params.append('txn_ids[]', id);
        });
        params.append('account_id', accountId);

        // Send POST request
        fetch('/bank-txns/bulk-skip', {
            method: 'POST',
            body: params,
            headers: {
                'Accept': 'text/html',
                'Content-Type': 'application/x-www-form-urlencoded',
            }
        })
        .then(response => {
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            return response.text();
        })
        .then(html => {
            // Replace the transactions section with the updated list
            const transactionsSection = document.getElementById('transactions');
            if (transactionsSection) {
                transactionsSection.outerHTML = html;
            }

            // Show success message
            const message = count === 1
                ? '1 transaction skipped successfully'
                : `${count} transactions skipped successfully`;

            if (window.Toast) {
                window.Toast.success(message);
            }

            // Clear selections and reinitialize
            selectedTxnIds.clear();
            setTimeout(() => {
                reinit();
            }, 50);
        })
        .catch(error => {
            if (window.Toast) {
                window.Toast.error('Failed to skip transactions: ' + error.message);
            } else {
                alert('Failed to skip transactions: ' + error.message);
            }
        });
    }

    // Initialize event listeners
    function init() {
        const elements = getElements();

        // Select-all checkbox
        if (elements.selectAllCheckbox) {
            elements.selectAllCheckbox.addEventListener('change', handleSelectAll);
        }

        // Individual checkboxes
        elements.txnCheckboxes.forEach(checkbox => {
            checkbox.addEventListener('change', handleCheckboxChange);
        });

        // Bulk skip button
        const bulkSkipBtn = document.querySelector('.bulk-skip-btn');
        if (bulkSkipBtn) {
            bulkSkipBtn.removeEventListener('click', handleBulkSkip); // Remove old listener
            bulkSkipBtn.addEventListener('click', handleBulkSkip);
        }

        // Initial UI state
        updateSelectionUI();
    }

    // Reinitialize after HTMX swaps (when table is updated)
    function reinit() {
        // Clear old selections for removed rows
        const elements = getElements();
        const currentTxnIds = new Set(
            Array.from(elements.txnCheckboxes).map(cb => cb.dataset.txnId)
        );

        // Remove selections for rows that no longer exist
        selectedTxnIds.forEach(id => {
            if (!currentTxnIds.has(id)) {
                selectedTxnIds.delete(id);
            }
        });

        // Update checkboxes to match state
        elements.txnCheckboxes.forEach(checkbox => {
            const txnId = checkbox.dataset.txnId;
            if (selectedTxnIds.has(txnId) && !checkbox.disabled) {
                checkbox.checked = true;
            }
        });

        init();
    }

    // Public API
    window.BulkEdit = {
        init: init,
        reinit: reinit,
        getSelectedIds: () => Array.from(selectedTxnIds),
        clearSelection: () => {
            selectedTxnIds.clear();
            const elements = getElements();
            elements.txnCheckboxes.forEach(cb => { cb.checked = false; });
            if (elements.selectAllCheckbox) {
                elements.selectAllCheckbox.checked = false;
                elements.selectAllCheckbox.indeterminate = false;
            }
            updateSelectionUI();
        }
    };

    // Initialize on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // Reinitialize after HTMX swaps
    document.body.addEventListener('htmx:afterSwap', function(event) {
        // Only reinit if the swap affected the transactions section
        if (event.detail.target.id === 'transactions' ||
            event.detail.target.classList.contains('txn-row')) {
            setTimeout(reinit, 10); // Small delay to ensure DOM is updated
        }
    });

})();
