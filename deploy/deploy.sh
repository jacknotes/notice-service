#!/usr/bin/env bash
# Notice Service · 服务器一键部署（构建镜像 → 换 tag → 重启 → 验证）
#
# 用法（在本地 Windows PowerShell 里）：
#   wsl -e bash /mnt/f/project/notice-service/deploy/deploy.sh [选项]
#
# 用法（在服务器上）：
#   bash deploy/deploy.sh [选项]
#
# 选项：
#   --host <user@host>     远程部署（默认 root@tencent.markli.cn）
#   --local                在本地/当前机器执行（不 ssh）
#   --dir <path>           服务器项目目录（默认 /opt/notice-service）
#   --compose <file>       compose 文件（默认 docker-compose.quickstart.yml）
#   --skip-build           跳过构建，只用已有镜像重启
#   --rollback             回滚到上一个镜像 tag
#   --check                只做健康检查，不做任何变更
#   -h, --help             显示帮助
#
# 设计原则（血泪教训，勿删）：
#   1. 只改 compose 的 image 一行，绝不动 .env / MySQL / 数据卷
#   2. 构建前记录「回滚点」（当前 commit + 当前镜像 tag）
#   3. 部署后必须验证：容器 healthy + /api/health 200 + 日志无 Fatal
#   4. 失败自动回滚（除非 --no-rollback）
set -uo pipefail

# ── 默认配置 ────────────────────────────────────────────────────────
REMOTE="root@tencent.markli.cn"
REMOTE_DIR="/opt/notice-service"
COMPOSE_FILE="docker-compose.quickstart.yml"
LOCAL_MODE=0
SKIP_BUILD=0
DO_ROLLBACK=0
CHECK_ONLY=0
AUTO_ROLLBACK=1

log()  { printf '\033[36m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[32m  ✓\033[0m %s\n' "$*"; }
warn() { printf '\033[33m  !\033[0m %s\n' "$*"; }
err()  { printf '\033[31m  ✗\033[0m %s\n' "$*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --host) REMOTE="$2"; shift 2 ;;
    --local) LOCAL_MODE=1; shift ;;
    --dir) REMOTE_DIR="$2"; shift 2 ;;
    --compose) COMPOSE_FILE="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --rollback) DO_ROLLBACK=1; shift ;;
    --check) CHECK_ONLY=1; shift ;;
    --no-rollback) AUTO_ROLLBACK=0; shift ;;
    -h|--help) sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) err "未知参数: $1"; exit 1 ;;
  esac
done

# ── 执行封装：本地直接跑，远程走 ssh ────────────────────────────────
run() {
  if [ "$LOCAL_MODE" = "1" ]; then
    bash -c "$1"
  else
    ssh -o BatchMode=yes -o ConnectTimeout=15 "$REMOTE" "$1"
  fi
}

# 把 stdin 脚本投递到远端执行（避免多层引号被吃掉——这是本 skill 的硬约束）
run_script() {
  if [ "$LOCAL_MODE" = "1" ]; then
    bash -s
  else
    ssh -o BatchMode=yes -o ConnectTimeout=15 "$REMOTE" 'cat > /tmp/_deploy_step.sh && bash /tmp/_deploy_step.sh'
  fi
}

# ── 0. 连通性与前置检查 ────────────────────────────────────────────
log "0/7 前置检查"
if [ "$LOCAL_MODE" != "1" ]; then
  if ! run "echo ok" >/dev/null 2>&1; then
    err "无法 SSH 到 $REMOTE（检查密钥/网络）"; exit 1
  fi
  ok "SSH 连通 $REMOTE"
fi

if ! run "cd $REMOTE_DIR && test -f $COMPOSE_FILE" >/dev/null 2>&1; then
  err "$REMOTE_DIR/$COMPOSE_FILE 不存在"; exit 1
fi
ok "目录与 compose 文件存在"

# ── 1. 记录回滚点 ──────────────────────────────────────────────────
log "1/7 记录回滚点"
CUR_COMMIT=$(run "cd $REMOTE_DIR && git rev-parse --short HEAD")
CUR_IMAGE=$(run "cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE ps --format '{{.Image}}' 2>/dev/null | grep notice-service | head -1")
CUR_TAG=$(run "cd $REMOTE_DIR && grep -E '^\s+image: jacknotes/notice-service:' $COMPOSE_FILE | head -1 | sed 's/.*notice-service://'")
echo "    commit      : $CUR_COMMIT"
echo "    运行中镜像  : ${CUR_IMAGE:-<none>}"
echo "    compose tag : $CUR_TAG"
printf '%s\n' "$CUR_TAG" > /tmp/_notice_prev_tag
ok "回滚点已记录"

if [ "$CHECK_ONLY" = "1" ]; then
  log "--check 模式：仅健康检查"
  run "cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE ps --format '{{.Name}} {{.Status}}'"
  run "curl -s -o /dev/null -w 'health=%{http_code}\n' http://127.0.0.1:8081/api/health"
  exit 0
fi

# ── 2. 回滚模式 ────────────────────────────────────────────────────
if [ "$DO_ROLLBACK" = "1" ]; then
  log "回滚到 $CUR_TAG"
  run_script <<EOF
set -e
cd $REMOTE_DIR
sed -i "s#image: jacknotes/notice-service:.*#image: jacknotes/notice-service:$CUR_TAG#" $COMPOSE_FILE
docker compose -f $COMPOSE_FILE up -d
sleep 12
docker compose -f $COMPOSE_FILE ps --format '{{.Name}} {{.Image}} {{.Status}}'
curl -s -o /dev/null -w 'health=%{http_code}\n' http://127.0.0.1:8081/api/health
EOF
  exit $?
fi

# ── 3. 拉取最新代码 ────────────────────────────────────────────────
log "2/7 拉取最新代码"
run_script <<EOF
set -e
cd $REMOTE_DIR
git fetch origin
git merge --ff-only origin/main
echo "HEAD=\$(git rev-parse --short HEAD)"
echo "DESCRIBE=\$(git describe --tags --always | sed 's/-dirty//')"
EOF
if [ $? -ne 0 ]; then err "拉取代码失败（可能有本地改动，先检查 git status）"; exit 1; fi

VERSION=$(run "cd $REMOTE_DIR && git describe --tags --always | sed 's/-dirty//'")
ok "版本号 $VERSION"

# ── 4. 构建镜像 ────────────────────────────────────────────────────
if [ "$SKIP_BUILD" = "1" ]; then
  warn "跳过构建（--skip-build）"
else
  log "3/7 构建镜像 $VERSION（约 1–3 分钟）"
  run_script <<EOF
set -e
cd $REMOTE_DIR
IMAGE_PREFIX=\$(grep -E '^IMAGE_PREFIX=' .env 2>/dev/null | tail -1 | cut -d= -f2-)
[ -z "\$IMAGE_PREFIX" ] && IMAGE_PREFIX=""
docker build \
  --build-arg BUILD_VERSION=$VERSION \
  --build-arg IMAGE_PREFIX="\$IMAGE_PREFIX" \
  --build-arg NPM_REGISTRY=https://registry.npmmirror.com \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  -t jacknotes/notice-service:$VERSION \
  -t jacknotes/notice-service:latest \
  . > /tmp/notice-build.log 2>&1
echo "BUILD_EXIT=\$?"
tail -3 /tmp/notice-build.log
EOF
  if [ $? -ne 0 ]; then
    err "构建失败，日志：/tmp/notice-build.log（服务器上）"
    run "tail -30 /tmp/notice-build.log"
    exit 1
  fi
  ok "镜像已构建"
fi

# ── 5. 验证镜像内容（防止构建出空壳/旧前端）─────────────────────────
log "4/7 验证镜像"
run_script <<EOF
set -e
cd $REMOTE_DIR
echo -n "  版本注入: "; docker run --rm --entrypoint /app/notice-service jacknotes/notice-service:$VERSION --version
echo -n "  前端产物: "; docker run --rm --entrypoint sh jacknotes/notice-service:$VERSION -c 'ls /app/web/dist/index.html >/dev/null 2>&1 && echo ok || echo MISSING'
EOF

# ── 6. 更新 compose tag 并重启 ─────────────────────────────────────
log "5/7 更新 compose 镜像 tag 并重启"
run_script <<EOF
set -e
cd $REMOTE_DIR
sed -i "s#image: jacknotes/notice-service:.*#image: jacknotes/notice-service:$VERSION#" $COMPOSE_FILE
grep -n 'image: jacknotes' $COMPOSE_FILE
docker compose -f $COMPOSE_FILE up -d
EOF
if [ $? -ne 0 ]; then err "重启失败"; exit 1; fi
ok "容器已重启（MySQL 不会被重建）"

# ── 7. 健康验证 ────────────────────────────────────────────────────
log "6/7 健康验证（等待 15s）"
sleep 15
STATUS=$(run "cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE ps --format '{{.Name}}|{{.Status}}' | grep 'notice-service-notice-service' | head -1")
HEALTH=$(run "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8081/api/health")
FATAL=$(run "cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE logs --tail=50 notice-service 2>&1 | grep -cE 'Fatal|panic|拒绝启动' || true")

echo "    容器状态: $STATUS"
echo "    健康检查: $HEALTH"
echo "    Fatal 数: $FATAL"

FAIL=0
case "$STATUS" in *Restarting*|*Exit*) FAIL=1 ;; esac
[ "$HEALTH" != "200" ] && FAIL=1
[ "${FATAL:-0}" != "0" ] && FAIL=1

if [ "$FAIL" = "0" ]; then
  ok "部署成功：$VERSION"
  log "7/7 完成"
  exit 0
fi

err "部署后验证失败"
run "cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE logs --tail=25 notice-service 2>&1"

if [ "$AUTO_ROLLBACK" = "1" ]; then
  warn "自动回滚到 $CUR_TAG"
  run_script <<EOF
cd $REMOTE_DIR
sed -i "s#image: jacknotes/notice-service:.*#image: jacknotes/notice-service:$CUR_TAG#" $COMPOSE_FILE
docker compose -f $COMPOSE_FILE up -d
sleep 12
docker compose -f $COMPOSE_FILE ps --format '{{.Name}} {{.Image}} {{.Status}}'
curl -s -o /dev/null -w 'health=%{http_code}\n' http://127.0.0.1:8081/api/health
EOF
  warn "已回滚，请查看上方日志定位原因"
fi
exit 1
