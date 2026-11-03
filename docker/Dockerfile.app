# Build extension and daemon from the same pinned source on the runtime architecture.
FROM --platform=$TARGETPLATFORM node:24-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e AS browserskill
WORKDIR /build
# Use the same optional Debian mirror as the Go builder and runtime stages.
ARG APK_MIRROR_ARG
# This base image has no CA certificates. Install them from the original HTTP
# Debian source before swapping in a mirror: mirrors.tencent.com redirects
# apt to HTTPS, and that handshake fails without a trust store.
RUN if [ -n "$APK_MIRROR_ARG" ]; then \
        apt-get update && \
        apt-get install -y --no-install-recommends ca-certificates && \
        rm -rf /var/lib/apt/lists/* && \
        sed -i "s@deb.debian.org@${APK_MIRROR_ARG}@g" /etc/apt/sources.list.d/debian.sources; \
    fi && \
    apt-get update && \
    apt-get install -y --no-install-recommends git python3 ca-certificates curl build-essential cmake pkg-config && \
    rm -rf /var/lib/apt/lists/*
ENV RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo
ENV PATH=/usr/local/cargo/bin:$PATH
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --profile minimal --default-toolchain stable
COPY scripts/build_browserskill.sh scripts/browserskill-release.json ./scripts/
ARG TARGETOS
ARG TARGETARCH
RUN bash scripts/build_browserskill.sh /opt/weknora/browserskill "${TARGETOS}/${TARGETARCH}"

# anydoc Rust 静态库（office 文档解析，cgo 链接进 Go 二进制）。
# 独立阶段：父层只依赖 third_party/anydoc-go 与脚本，且缓存 rust 工具链，
# 因此"只改 Go 源码"不会使本阶段失效，anydoc 层保持 CACHED（不再每次装 Rust/重编）。
FROM --platform=$TARGETPLATFORM node:24-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e AS anydoc-lib
WORKDIR /app
ARG APK_MIRROR_ARG
RUN if [ -n "$APK_MIRROR_ARG" ]; then \
        apt-get update && \
        apt-get install -y --no-install-recommends ca-certificates && \
        rm -rf /var/lib/apt/lists/* && \
        sed -i "s@deb.debian.org@${APK_MIRROR_ARG}@g" /etc/apt/sources.list.d/debian.sources; \
    fi && \
    apt-get update && \
    apt-get install -y --no-install-recommends build-essential pkg-config cmake curl git python3 ca-certificates && \
    rm -rf /var/lib/apt/lists/*
ENV RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo
ENV PATH=/usr/local/cargo/bin:$PATH
# 工具链 + 依赖仓库缓存在 /usr/local/{rustup,cargo}：首跑装一次，后续命中缓存、不重装。
RUN --mount=type=cache,target=/usr/local/rustup \
    --mount=type=cache,target=/usr/local/cargo \
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --profile minimal --default-toolchain stable
COPY third_party/anydoc-go/ ./third_party/anydoc-go/
COPY scripts/build-anydoc-lib.sh ./scripts/build-anydoc-lib.sh
RUN --mount=type=cache,target=/usr/local/rustup \
    --mount=type=cache,target=/usr/local/cargo \
    ./scripts/build-anydoc-lib.sh

# Build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /app

# 通过构建参数接收敏感信息
ARG GOPRIVATE_ARG
ARG GOPROXY_ARG
ARG GOSUMDB_ARG=off
ARG APK_MIRROR_ARG

# 设置Go环境变量
ENV GOPRIVATE=${GOPRIVATE_ARG}
ENV GOPROXY=${GOPROXY_ARG}
ENV GOSUMDB=${GOSUMDB_ARG}

# Install dependencies
RUN if [ -n "$APK_MIRROR_ARG" ]; then \
        sed -i "s@deb.debian.org@${APK_MIRROR_ARG}@g" /etc/apt/sources.list.d/debian.sources; \
    fi && \
    apt-get update && \
    apt-get install -y git build-essential libsqlite3-dev curl

# Install migrate tool
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Copy go mod files. go.mod replace-points anydoc at ./third_party/anydoc-go,
# so that module's go.mod must exist before `go mod download`.
COPY go.mod go.sum ./
COPY third_party/anydoc-go/go.mod third_party/anydoc-go/go.mod
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/download cmd/download
RUN go run cmd/download/duckdb/duckdb.go
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod bash ./scripts/copy-licenses.sh /license-bundle

# Get version and commit info for build injection
ARG VERSION_ARG
ARG COMMIT_ID_ARG
ARG BUILD_TIME_ARG
ARG GO_VERSION_ARG

# Set build-time variables
ENV VERSION=${VERSION_ARG}
ENV COMMIT_ID=${COMMIT_ID_ARG}
ENV BUILD_TIME=${BUILD_TIME_ARG}
ENV GO_VERSION=${GO_VERSION_ARG}

# anydoc 静态库由独立 anydoc-lib 阶段产出（该阶段只依赖 third_party/anydoc-go），
# 只改 Go 源码时它保持 CACHED；这里直接 COPY 其产物 .a，不再就地装 Rust/重编。
ARG WITH_ANYDOC=1
COPY --from=anydoc-lib /app/third_party/anydoc-go/lib/linux_amd64_gnu/libanydoc_go.a third_party/anydoc-go/lib/linux_amd64_gnu/libanydoc_go.a

# Build the application with version info
# /root/.cache/go-build 持久化 Go 编译缓存：纯 Go 改动只重编受影响包并重新链接，
# 由“整包全量重编”降为增量（秒级~十几秒），不再每次几分钟。
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$WITH_ANYDOC" = "1" ]; then \
        make build-prod GO_BUILD_TAGS=anydoc; \
    else \
        make build-prod; \
    fi
RUN --mount=type=cache,target=/go/pkg/mod cp -r /go/pkg/mod/github.com/yanyiwu/ /app/yanyiwu/

# Final stage
FROM debian:12.12-slim

WORKDIR /app

ARG APK_MIRROR_ARG

# Pairing derives the gateway URL from the user's page origin by default.
ENV BROWSERSKILL_BINARY=/opt/weknora/browserskill/bsk \
    BROWSERSKILL_EXTENSION_PATH=/opt/weknora/browserskill/browser-skill-weknora-0.3.1.zip
COPY --from=browserskill /opt/weknora/browserskill /opt/weknora/browserskill

# Create a non-root user first
RUN useradd -m -s /bin/bash appuser

# First, install ca-certificates without mirror to ensure HTTPS works
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Then switch to mirror if specified and install other packages
RUN if [ -n "$APK_MIRROR_ARG" ]; then \
        sed -i "s@deb.debian.org@${APK_MIRROR_ARG}@g" /etc/apt/sources.list.d/debian.sources; \
    fi && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        build-essential postgresql-client default-mysql-client tzdata sed curl bash vim wget \
        libsqlite3-0 \
        python3 python3-pip python3-dev libffi-dev libssl-dev \
        nodejs npm \
        chromium pandoc \
        texlive-xetex texlive-lang-chinese texlive-fonts-recommended \
        libfontconfig1 fonts-noto-cjk fonts-noto-cjk-extra fonts-noto-color-emoji fonts-liberation \
        gosu \
        ffmpeg && \
    python3 -m pip install --break-system-packages --upgrade pip setuptools wheel && \
    mkdir -p /home/appuser/.local/bin && \
    curl -LsSf https://astral.sh/uv/install.sh | CARGO_HOME=/home/appuser/.cargo UV_INSTALL_DIR=/home/appuser/.local/bin sh && \
    chown -R appuser:appuser /home/appuser && \
    ln -sf /home/appuser/.local/bin/uvx /usr/local/bin/uvx && \
    chmod +x /usr/local/bin/uvx && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

ENV WEKNORA_EXPORT_CHROME_BIN=/usr/bin/chromium

# Create data directories and set permissions
RUN mkdir -p /data/files && \
    chown -R appuser:appuser /app /data/files

# Copy migrate tool from builder stage
COPY --from=builder /go/bin/migrate /usr/local/bin/
COPY --from=builder /app/yanyiwu/ /go/pkg/mod/github.com/yanyiwu/

# Copy the binary from the builder stage
COPY --from=builder /app/config ./config
COPY --from=builder /app/scripts ./scripts
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/dataset/samples ./dataset/samples
COPY --from=builder /root/.duckdb /home/appuser/.duckdb
COPY --from=builder /app/WeKnora .
COPY --from=builder /license-bundle/ ./

# Copy and make entrypoint script executable
COPY --from=builder /app/scripts/docker-entrypoint.sh ./scripts/docker-entrypoint.sh

# Make scripts executable
RUN chmod +x ./scripts/*.sh

# Expose ports
EXPOSE 8080


ENTRYPOINT ["./scripts/docker-entrypoint.sh"]
CMD ["./WeKnora"]
