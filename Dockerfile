# gitd and the git it runs. The image serves by default (`docker run … ghcr.io/dooservice/gitd`)
# and is also the command line: `docker exec <container> gitd prepare …`.
FROM --platform=$BUILDPLATFORM golang:1.27.2-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev TARGETOS TARGETARCH GITD_LOGIN_CLIENT_ID GITD_LOGIN_APP
# Cross-compiled on the build machine: no emulation for Go, whatever the image's platform.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION} \
      -X github.com/dooservice/gitd/internal/config.loginClientID=${GITD_LOGIN_CLIENT_ID} \
      -X github.com/dooservice/gitd/internal/config.loginApp=${GITD_LOGIN_APP}" -o /gitd ./cmd/gitd \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /gitd-mount ./cmd/gitd-mount

FROM debian:12-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends git openssh-client ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 1000 --user-group --home-dir /var/lib/gitd gitd \
 && mkdir -p /var/lib/gitd/workspaces /run/gitd /etc/gitd \
 && chown -R gitd:gitd /var/lib/gitd /run/gitd
COPY --from=build /gitd /gitd-mount /usr/local/bin/
USER gitd
ENV GITD_SOCKET=/run/gitd/gitd.sock
VOLUME ["/var/lib/gitd", "/run/gitd"]
ENTRYPOINT ["gitd"]
