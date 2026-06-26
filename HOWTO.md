# HOW TO

## Reproxy config

```yml
default: # the same as * (catch-all) server
  - { route: "^/api/(.*)", dest: "http://127.0.0.1:5002/api/$1" }
  - { route: "http://ynab-helper-dev.com/api/", dest: "http://localhost:5002/$1" }
```

Run:

```bash
sudo ./reproxy --file.enabled --file.name=config.yml
```

## FrontEnd call


```js
let url = `http://ynab-helper-dev.com/api` + `/v1/transactions`;
```