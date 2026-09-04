#!/usr/bin/env bash
# Notice Service · 发布到 Docker Hub
#
# 构建当前代码为镜像，打上「版本号 tag + latest tag」并推送到 Docker Hub，
# 保证 hub.docker.com 上 jacknotes/notice-service:latest 始终指向最新构建。
#
# 版本号取自 `git describe --tags --always`（与 Makefile VERSION 一致），
# 也作为注入二进制的 buildVersion 与镜像 tag。dirty 工作区会带上 -dirty 后缀，
# 提示当前构建并非干净提交（可配合 --force 强制发布）。
#
# 用法：
#   ./deploy/publish.sh [--force]
#   make publish            # 等价（可选 FORCE=1）
#
# 可选环境变量（国内网络构建加速，与 Dockerfile ARG 对应）：
#   IMAGE_PREFIX  基础镜像前缀，如 docker.m.daocloud.io/library/（默认空=官方 Docker Hub）
#   NPM_REGISTRY  npm 源，默认 https://registry.npmmirror.com
#   GOPROXY       Go 模块代理，默认 https://goproxy.cn,direct
#   REGISTRY      目标镜像仓库前缀，默认 jacknotes/notice-service
#
# 前置：已 `docker login`（推送目标仓库需登录态）。

set -euo pipefail

REGISTRY="${REGISTRY:-jacknotes/notice-service}"
IMAGE_PREFIX_ARG=""
[ -n "${IMAGE_PREFIX:-}" ] && IMAGE_PREFIX_ARG="--build-arg IMAGE_PREFIX=${IMAGE_PREFIX}"

VERSION="$(git describe --tags --always --dirty)"
case "$VERSION" in
  *-dirty)
    if [ "${1:-}" != "--force" ]; then
      echo "错误：工作区有未提交改动（版本号 ${VERSION}）。" >&2
      echo "请先提交，或用 --force 强制发布（不推荐，latest 会指向含未提交改动的构建）。" >&2
      exit 1
    fi
    echo "警告：--force 强制发布 dirty 工作区，版本号 ${VERSION}。" >&2
    ;;
esac

echo "==> 发布版本：${VERSION}"
echo "==> 目标仓库：${REGISTRY}"

echo "==> 构建镜像（BUILD_VERSION=${VERSION}）..."
docker build \
  --build-arg BUILD_VERSION="${VERSION}" \
  ${IMAGE_PREFIX_ARG:-} \
  --build-arg NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}" \
  --build-arg GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
  -t "${REGISTRY}:${VERSION}" \
  -t "${REGISTRY}:latest" \
  .

echo "==> 推送 ${REGISTRY}:${VERSION} ..."
docker push "${REGISTRY}:${VERSION}"

echo "==> 推送 ${REGISTRY}:latest ..."
docker push "${REGISTRY}:latest"

echo
echo "✅ 发布完成："
echo "   ${REGISTRY}:${VERSION}"
echo "   ${REGISTRY}:latest → ${VERSION}（latest 已指向本次最新构建）"
