ARG TZ=Europe/Warsaw

FROM umputun/baseimage:buildgo-latest AS build-backend

ARG CI
ARG GIT_BRANCH
ARG SKIP_TEST
ARG GITHUB_SHA

ADD . /build/ynab-helper
WORKDIR /build/ynab-helper

RUN \
    go mod tidy

# run tests and linters
RUN \
    if [ -z "$SKIP_TEST" ] ; then \
    go test -timeout=30s  ./... && \
    golangci-lint run ; \
    else echo "skip tests and linter" ; fi

RUN \
    if [ -z "$CI" ] ; then \
    echo "runs outside of CI" && version=$(git rev-parse --abbrev-ref HEAD)-$(git log -1 --format=%h)-$(date +%Y%m%dT%H:%M:%S); \
    else version=${GIT_BRANCH}-${GITHUB_SHA:0:7}-$(date +%Y%m%dT%H:%M:%S); fi && \
    echo "version=$version" && \
    go build -o /build/ynab-helper.bin -ldflags "-X main.revision=${version} -s -w" ./cmd/ynab-helper


FROM umputun/baseimage:app-latest

ARG TZ=Europe/Warsaw
ENV TZ=${TZ}

COPY --from=build-backend /build/ynab-helper.bin /srv/ynab-helper
COPY --from=build-backend /build/ynab-helper/ui/static /srv/ui/static/

WORKDIR /srv
EXPOSE 8080


CMD ["/srv/ynab-helper"]
ENTRYPOINT ["/init.sh"]
