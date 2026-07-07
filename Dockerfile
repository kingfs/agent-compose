ARG REGISTRY_MIRROR=docker.io
ARG GOPROXY=https://goproxy.cn,direct

FROM ${REGISTRY_MIRROR}/library/golang:1-alpine AS golang-toolchain

FROM ${REGISTRY_MIRROR}/library/rust:1.88-trixie AS boxlite-build
ARG BOXLITE_VERSION=v0.9.7
ARG TARGETARCH
ARG GOPROXY
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV ALL_PROXY=${ALL_PROXY}
ENV NO_PROXY=${NO_PROXY}
ENV no_proxy=${NO_PROXY}
ENV GOPROXY=${GOPROXY}
COPY --from=golang-toolchain /usr/local/go /usr/local/go
ENV RUSTUP_TOOLCHAIN=1.88.0
ENV PATH=/usr/local/cargo/bin:/usr/local/go/bin:${PATH}
RUN if [ -f /etc/apt/sources.list ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list;     fi &&     if [ -f /etc/apt/sources.list.d/debian.sources ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources;     fi
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    bc \
    binutils \
    bison \
    build-essential \
    ca-certificates \
    cmake \
    curl \
    file \
    flex \
    gperf \
    git \
    libbz2-dev \
    libcap-dev \
    libclang-dev \
    libelf-dev \
    libssl-dev \
    llvm \
    meson \
    musl-tools \
    ninja-build \
    patchelf \
    pkg-config \
    protobuf-compiler \
    python3 \
    python3-pip \
    python3-pyelftools \
    python3-venv \
    tar \
    wget && \
    rm -rf /var/lib/apt/lists/*
RUN <<'EOF' bash
set -euo pipefail

target_arch="${TARGETARCH:-$(dpkg --print-architecture)}"
case "${target_arch}" in
  amd64) MUSL_ARCH=x86_64 ;;
  arm64) MUSL_ARCH=aarch64 ;;
  *) echo "unsupported BoxLite target arch: ${target_arch}" >&2; exit 1 ;;
esac

if ! command -v "${MUSL_ARCH}-linux-musl-gcc" >/dev/null 2>&1; then
  ln -sf "$(command -v musl-gcc)" "/usr/local/bin/${MUSL_ARCH}-linux-musl-gcc"
fi

for attempt in 1 2 3 4 5; do
  git -c http.version=HTTP/1.1 clone --branch "${BOXLITE_VERSION}" --depth 1 --recurse-submodules --shallow-submodules https://github.com/boxlite-ai/boxlite.git /tmp/boxlite-src && break
  rm -rf /tmp/boxlite-src
  if [ "$attempt" = 5 ]; then
    exit 1
  fi
  sleep $((attempt * 5))
done

cd /tmp/boxlite-src
sed -i 's/\.args(\["-fsSL", "-o", dest.to_str().unwrap(), url\])/.args(["--http1.1", "--retry", "5", "--retry-all-errors", "--retry-delay", "2", "-fsSL", "-o", dest.to_str().unwrap(), url])/' src/deps/libkrun-sys/build.rs
bz2_static="/usr/lib/$(dpkg-architecture -qDEB_HOST_MULTIARCH)/libbz2.a"
test -f "${bz2_static}"
sed -i '/cargo:rustc-link-lib=static=gvproxy/a\        println!("cargo:rustc-link-lib=static=bz2");' src/deps/libgvproxy-sys/build.rs
for attempt in 1 2 3 4 5; do
  (cd src/deps/libgvproxy-sys/gvproxy-bridge && go mod download) && break
  if [ "$attempt" = 5 ]; then
    exit 1
  fi
  sleep $((attempt * 5))
done

cargo build --release -p boxlite-c
bash scripts/build/build-runtime.sh --profile release --dest-dir /out/runtime

mkdir -p /out/include /out/lib
cp sdks/c/include/boxlite.h /out/include/boxlite.h
cp -a target/release/libboxlite.* /out/lib/

file /out/runtime/boxlite-shim | tee /tmp/boxlite-shim.file
if grep -q 'dynamically linked' /tmp/boxlite-shim.file; then
  echo "invalid BoxLite shim build: expected static shim" >&2
  exit 1
fi

cat >/tmp/agent-compose-dlopen-libkrunfw.c <<'C_EOF'
#include <dlfcn.h>
#include <stdio.h>
int main(void) {
  void *handle = dlopen("libkrunfw.so.5", RTLD_NOW | RTLD_LOCAL);
  if (!handle) {
    fprintf(stderr, "dlopen libkrunfw.so.5 failed: %s\n", dlerror());
    return 1;
  }
  if (!dlsym(handle, "krunfw_get_kernel")) {
    fprintf(stderr, "dlsym krunfw_get_kernel failed: %s\n", dlerror());
    return 1;
  }
  return 0;
}
C_EOF

gcc -O2 -static -o /tmp/agent-compose-dlopen-libkrunfw /tmp/agent-compose-dlopen-libkrunfw.c -ldl
LD_LIBRARY_PATH=/out/runtime /tmp/agent-compose-dlopen-libkrunfw
EOF

# Fetch the prebuilt microsandbox artifacts for the target architecture. The Go
# FFI library (libmicrosandbox_go_ffi) ships as a release asset, so there is no
# need to build it from source with a Rust toolchain — we just download it
# alongside msb, agentd and libkrunfw and verify everything against the
# published checksums. This keeps the FFI lib in lockstep with the
# microsandbox/sdk/go module pinned in go.mod.
FROM ${REGISTRY_MIRROR}/library/debian:bookworm AS microsandbox-fetch
ARG MICROSANDBOX_VERSION=v0.6.4
ARG TARGETARCH
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV ALL_PROXY=${ALL_PROXY}
ENV NO_PROXY=${NO_PROXY}
ENV no_proxy=${NO_PROXY}
RUN if [ -f /etc/apt/sources.list ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list;     fi &&     if [ -f /etc/apt/sources.list.d/debian.sources ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources;     fi
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl binutils tar &&     rm -rf /var/lib/apt/lists/*
RUN set -e;     target_arch="${TARGETARCH:-$(dpkg --print-architecture)}";     case "${target_arch}" in       amd64) MICROSANDBOX_ARCH=x86_64 ;;       arm64) MICROSANDBOX_ARCH=aarch64 ;;       *) echo "unsupported Microsandbox target arch: ${target_arch}" >&2; exit 1 ;;     esac;     base="https://github.com/superradcompany/microsandbox/releases/download/${MICROSANDBOX_VERSION}";     mkdir -p /tmp/microsandbox/extract /out/bin /out/lib;     cd /tmp/microsandbox;     curl --http1.1 --retry 5 --retry-all-errors --retry-delay 2 -fsSL -O "${base}/microsandbox-linux-${MICROSANDBOX_ARCH}.tar.gz";     curl --http1.1 --retry 5 --retry-all-errors --retry-delay 2 -fsSL -O "${base}/agentd-${MICROSANDBOX_ARCH}";     curl --http1.1 --retry 5 --retry-all-errors --retry-delay 2 -fsSL -O "${base}/libmicrosandbox_go_ffi-linux-${target_arch}.so";     curl --http1.1 --retry 5 --retry-all-errors --retry-delay 2 -fsSL -O "${base}/checksums.sha256";     sha256sum -c --ignore-missing checksums.sha256;     tar -xzf "microsandbox-linux-${MICROSANDBOX_ARCH}.tar.gz" -C /tmp/microsandbox/extract;     install -m755 /tmp/microsandbox/extract/msb /out/bin/msb;     install -m755 "agentd-${MICROSANDBOX_ARCH}" /out/bin/agentd;     krunfw="$(find /tmp/microsandbox/extract -maxdepth 1 -type f -name 'libkrunfw.so.*' | sort | tail -n 1)";     test -n "${krunfw}";     krunfw_name="$(basename "${krunfw}")";     install -m644 "${krunfw}" "/out/lib/${krunfw_name}";     ln -sf "${krunfw_name}" /out/lib/libkrunfw.so.5;     ln -sf libkrunfw.so.5 /out/lib/libkrunfw.so;     install -m644 "libmicrosandbox_go_ffi-linux-${target_arch}.so" /out/lib/libmicrosandbox_go_ffi.so;     strip --strip-unneeded /out/lib/libmicrosandbox_go_ffi.so 2>/dev/null || true

FROM ${REGISTRY_MIRROR}/library/debian:bookworm AS go-build
ARG VERSION=0
ARG TARGETARCH
ARG GOPROXY
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV ALL_PROXY=${ALL_PROXY}
ENV NO_PROXY=${NO_PROXY}
ENV no_proxy=${NO_PROXY}
RUN if [ -f /etc/apt/sources.list ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list;     fi &&     if [ -f /etc/apt/sources.list.d/debian.sources ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources;     fi
RUN apt-get update && apt-get install -y --no-install-recommends build-essential ca-certificates curl git tar && rm -rf /var/lib/apt/lists/*
COPY --from=golang-toolchain /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:${PATH}
WORKDIR /app
COPY --from=boxlite-build /out /app/build/boxlite
COPY go.mod go.sum ./
RUN go env -w GOPROXY="${GOPROXY}" && go mod download
COPY cmd ./cmd
COPY pkg ./pkg
COPY assets ./assets
COPY proto ./proto
RUN target_arch="${TARGETARCH:-$(dpkg --print-architecture)}" && CGO_ENABLED=1 GOOS=linux GOARCH=${target_arch} go build -ldflags "-X agent-compose/pkg/config.BuildVersion=${VERSION}" -tags 'netgo,osusergo,boxlitecgo' -o /out/agent-compose ./cmd/agent-compose

FROM scratch AS agent-compose-artifact
COPY --from=go-build /out/agent-compose /out/agent-compose

FROM ${REGISTRY_MIRROR}/library/debian:trixie-slim
RUN if [ -f /etc/apt/sources.list ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list;     fi &&     if [ -f /etc/apt/sources.list.d/debian.sources ]; then       sed -i -e 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources &&       sed -i -e 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources;     fi
RUN apt-get update &&     apt-get install -y --no-install-recommends ca-certificates git python3 tini tzdata e2fsprogs &&     rm -rf /var/lib/apt/lists/*
RUN ln -sfv /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone
WORKDIR /app
COPY --from=go-build /out/agent-compose /app/agent-compose
RUN ln -sf /app/agent-compose /usr/local/bin/agent-compose
COPY --from=boxlite-build /out/runtime /app/boxlite/runtime
COPY --from=microsandbox-fetch /out /app/microsandbox
ENV RUNTIME_DRIVER=docker
ENV DATA_ROOT=/data
ENV SESSION_ROOT=/data/sessions
ENV HTTP_LISTEN=0.0.0.0:7410
ENV DEFAULT_IMAGE=debian:bookworm-slim
ENV GUEST_WORKSPACE=/workspace
ENV GUEST_STATE_ROOT=/data/state
ENV GUEST_RUNTIME_ROOT=/data/runtime
ENV GUEST_LOG_ROOT=/data/logs
ENV BOXLITE_RUNTIME_DIR=/app/boxlite/runtime
ENV MICROSANDBOX_HOME=/data/microsandbox
ENV MICROSANDBOX_MSB_PATH=/app/microsandbox/bin/msb
ENV MICROSANDBOX_LIB_PATH=/app/microsandbox/lib/libmicrosandbox_go_ffi.so
ENV LD_LIBRARY_PATH=/app/boxlite/runtime:/app/microsandbox/lib
ENTRYPOINT ["/usr/bin/tini", "--", "/app/agent-compose"]
CMD ["daemon"]
