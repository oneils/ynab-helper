// htmx v4 issues requests via fetch() instead of XMLHttpRequest, which removed
// `htmx:xhr:progress` with no equivalent upload-progress signal. The determinate
// progress bar is downgraded to an indeterminate one, shown for the duration of the
// request instead of tracking bytes uploaded.
htmx.on('#upload-bank-txns-form', 'htmx:before:request', function (evt) {
    if (evt.target.id === 'upload-bank-txns-form') {
        htmx.find('#progress').removeAttribute('value');
    }
});

// htmx:finally:request (not htmx:after:request) so the indicator is always
// cleared, including on network errors/aborts where after:request never fires.
htmx.on('#upload-bank-txns-form', 'htmx:finally:request', function (evt) {
    if (evt.target.id === 'upload-bank-txns-form') {
        document.querySelector('.progress-container').style.display = 'none';
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