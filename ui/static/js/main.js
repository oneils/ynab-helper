htmx.on('#upload-bank-txns-form', 'htmx:xhr:progress', function (evt) {
    if (evt.target.id === 'upload-bank-txns-form') {
        htmx.find('#progress').setAttribute('value', evt.detail.loaded / evt.detail.total * 100)
    }
});