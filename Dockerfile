# syntax=docker/dockerfile:1
# Toolchains match .nvmrc/go.mod; digests pin both native Linux platforms.
FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:8a32c8cf649a49915a66e81e7a609223c708d0adabf288ee3f4077d765542f85 AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY frontend/ ./
COPY internal/web/translation /src/internal/web/translation
RUN npm run build

FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS native-build-base
WORKDIR /app
RUN apk add --no-cache build-base lld binutils git python3 bash curl unzip
COPY . .
ENV CGO_ENABLED=1 CGO_CFLAGS="-D_LARGEFILE64_SOURCE"
# The kernel target is also used by release.yml. Run native musl tests in
# Alpine, whose compiler/libc profile matches the packaged static Cronet.
FROM native-build-base AS kernel
ARG HIDDIFY_VULNCHECK=0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    HIDDIFY_VULNCHECK="$HIDDIFY_VULNCHECK" SINGBOX_CRONET_LINKAGE=musl bash tools/build-hiddify-core.sh /kernel

FROM native-build-base AS builder
ARG TARGETARCH
ARG BUILD_COMMIT=unknown
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-w -s -X github.com/SawaMEN/3x-ui/v3/internal/config.buildCommit=$BUILD_COMMIT" -o build/x-ui .
RUN ./DockerInit.sh "$TARGETARCH"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    python3 tools/build-speed-components.py --component mieru --output /app/build/bin
COPY --from=kernel /kernel/ /app/build/bin/

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
ARG BUILD_COMMIT=unknown
LABEL org.opencontainers.image.revision=$BUILD_COMMIT
ENV TZ=Asia/Tehran
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata fail2ban bash curl openssl
COPY --from=builder /app/build/ /app/
COPY --from=builder /app/DockerEntrypoint.sh /app/
COPY --from=builder /app/x-ui.sh /usr/bin/x-ui
COPY --from=builder /app/internal/web/translation /app/internal/web/translation
RUN rm -f /etc/fail2ban/jail.d/alpine-ssh.conf \
    && cp /etc/fail2ban/jail.conf /etc/fail2ban/jail.local \
    && sed -i "s/^\\[ssh\\]$/&\\nenabled = false/; s/^\\[sshd\\]$/&\\nenabled = false/" /etc/fail2ban/jail.local \
    && sed -i "s/#allowipv6 = auto/allowipv6 = auto/g" /etc/fail2ban/fail2ban.conf \
    && chmod +x /app/DockerEntrypoint.sh /app/x-ui /usr/bin/x-ui
ENV XUI_IN_DOCKER="true" XUI_MAIN_FOLDER="/app" XUI_ENABLE_FAIL2BAN="true" \
    XUI_DB_TYPE="" XUI_DB_DSN=""
EXPOSE 2053
VOLUME ["/etc/x-ui"]
CMD ["./x-ui"]
ENTRYPOINT ["/app/DockerEntrypoint.sh"]
