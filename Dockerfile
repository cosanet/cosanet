FROM golang:1.25 AS builder

ARG BUILD_TIMESTAMP="1970-01-01T00:00:00+00:00"
ARG COMMIT_HASH="00000000-dirty"
ARG PROJECT_URL="https://github.com/cosanet/cosanet"
ARG VERSION="v0.0.0"

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN set -e \
    && export BUILDER="$(go version)" \
    && CGO_ENABLED=0 go build -ldflags="-X 'main.Version=${VERSION}' -X 'main.CommitHash=${COMMIT_HASH}' -X 'main.BuildTimestamp=${BUILD_TIMESTAMP}' -X 'main.Builder=${BUILDER}' -X 'main.ProjectURL=${PROJECT_URL}'" -o cosanet .


FROM debian:trixie-slim

ARG BUILD_TIMESTAMP="1970-01-01T00:00:00+00:00"
ARG COMMIT_HASH="00000000-dirty"
ARG PROJECT_URL="https://github.com/cosanet/cosanet"
ARG VERSION="v0.0.0"

LABEL org.opencontainers.image.source=${PROJECT_URL}
LABEL org.opencontainers.image.created=${BUILD_TIMESTAMP}
LABEL org.opencontainers.image.version=${VERSION}
LABEL org.opencontainers.image.revision=${COMMIT_HASH}

WORKDIR /app

COPY --from=builder /app/cosanet .

EXPOSE 9156

CMD ["/app/cosanet"]
