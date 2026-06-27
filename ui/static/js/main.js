htmx.on('#upload-bank-txns-form', 'htmx:xhr:progress', function (evt) {
    if (evt.target.id === 'upload-bank-txns-form') {
        htmx.find('#progress').setAttribute('value', evt.detail.loaded / evt.detail.total * 100)
    }
});

function updateActiveNav() {
    var path = window.location.pathname;
    document.querySelectorAll('.nav-item').forEach(function (link) {
        var href = new URL(link.href).pathname;
        var active = href === '/' ? path === '/' : path === href || path.startsWith(href + '/');
        link.classList.toggle('active', active);
    });
}

updateActiveNav();

// Highlight immediately on click — avoids URL timing race with hx-push-url.
document.addEventListener('click', function (e) {
    var navItem = e.target.closest('.nav-item[hx-get]');
    if (!navItem) return;
    document.querySelectorAll('.nav-item').forEach(function (l) { l.classList.remove('active'); });
    navItem.classList.add('active');
});

// Re-sync when user navigates with browser back/forward buttons.
window.addEventListener('popstate', updateActiveNav);