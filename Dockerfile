# syntax=docker/dockerfile:1.7

ARG DOCKER_HUB_REGISTRY=docker.io
ARG GCR_REGISTRY=gcr.io

# Pin image indexes so upstream tag updates do not invalidate cached builds.
# Update the digests explicitly when upgrading the base images.
FROM --platform=$BUILDPLATFORM ${DOCKER_HUB_REGISTRY}/library/node:24-bookworm-slim@sha256:01d566e8d94d8c619591d78652babfccd3c6992a3dffa9ed5ab39e11cf6c70e9 AS web-build

ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
ENV NPM_CONFIG_REGISTRY=${NPM_CONFIG_REGISTRY}

WORKDIR /src/web
COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --prefer-offline
COPY web/ ./
RUN npm run build

FROM ${DOCKER_HUB_REGISTRY}/library/golang:1.25-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437 AS go-build

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.org
ARG BUILD_MODE=release
ENV GOPROXY=${GOPROXY} \
  GOSUMDB=${GOSUMDB}

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY assets/ ./assets/
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build -trimpath -ldflags="-s -w -X main.buildMode=${BUILD_MODE}" -o /out/javboss ./cmd/server

FROM --platform=$BUILDPLATFORM ${DOCKER_HUB_REGISTRY}/library/alpine:3.23@sha256:f3d55900449227ae9e237ede521e7b665d53a253b42065ba90b9db9275e198ad AS ffmpeg-build

ARG TARGETARCH
ARG FFMPEG_RELEASE=n8.1.2-1

RUN set -eu; \
  case "${TARGETARCH}" in \
    amd64) \
      asset_arch="x64"; \
      ffmpeg_sha256="9eac5b2b5076db5ff853a6fa0dcd6b8de7d0cac8481eadda6c47cd935825f1ee"; \
      ffprobe_sha256="065d3c56926052a76e884c4e4b51b7d95248da9391ab7effdcca6b94ceab98cf" \
      ;; \
    arm64) \
      asset_arch="arm64"; \
      ffmpeg_sha256="6e7b1d7d1aa8c35e3fedd78a140aa0968717aeb7386ecfb0ee00773d9f0a4503"; \
      ffprobe_sha256="fd2aca1456f0261cabef4514b6d97a70fa342003347f51b39c473dd364328089" \
      ;; \
    *) \
      echo "unsupported target architecture: ${TARGETARCH}" >&2; \
      exit 1 \
      ;; \
  esac; \
  release_url="https://github.com/shaka-project/static-ffmpeg-binaries/releases/download/${FFMPEG_RELEASE}"; \
  wget -q -O /ffmpeg "${release_url}/ffmpeg-linux-${asset_arch}"; \
  echo "${ffmpeg_sha256}  /ffmpeg" | sha256sum -c -; \
  wget -q -O /ffprobe "${release_url}/ffprobe-linux-${asset_arch}"; \
  echo "${ffprobe_sha256}  /ffprobe" | sha256sum -c -; \
  chmod 0755 /ffmpeg /ffprobe

FROM ${GCR_REGISTRY}/distroless/base-debian12:latest@sha256:76b3162a31477bca4a245b836c624f4c4a1a3705e99b9003907d992bec2c4bca

WORKDIR /app
COPY --from=ffmpeg-build /ffmpeg ./internal/bin/ffmpeg
COPY --from=ffmpeg-build /ffprobe ./internal/bin/ffprobe
COPY --from=go-build /out/javboss ./javboss
COPY --from=web-build /src/web/dist ./web/dist

ENV JAVBOSS_CONTAINER=1 \
  JAVBOSS_HOST_PATH_PREFIX=1 \
  JAVBOSS_PROXY_HOST_GATEWAY=1

EXPOSE 17654
VOLUME ["/app/data"]

CMD ["./javboss"]
