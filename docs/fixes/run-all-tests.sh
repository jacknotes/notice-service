#!/usr/bin/env bash
# 一键跑全部 7 个「立即」条目的验收脚本
# 用法（Windows PowerShell）：
#   wsl -e bash /mnt/f/project/notice-service/docs/fixes/run-all-tests.sh
# 用法（已在 WSL 里）：
#   cd /mnt/f/project/notice-service && bash docs/fixes/run-all-tests.sh
set -uo pipefail

cd /mnt/f/project/notice-service || exit 1
DIR=$(dirname "$0")

echo "############################################################"
echo "#   notice-service 修复验收 —— 7 个立即条目"
echo "#   $(date '+%Y-%m-%d %H:%M:%S')"
echo "############################################################"

# 前置：服务必须在跑
if ! curl -s -o /dev/null -w '' http://127.0.0.1:8080/api/health 2>/dev/null; then
  echo "!! 服务没在 8080 运行，先启动："
  echo "   wsl -e bash -lc 'cd /mnt/f/project/notice-service && nohup env -u ENCRYPT_KEY -u JWT_SECRET -u STRICT_SECRETS PORT=8080 /tmp/notice-server > /tmp/notice.log 2>&1 &'"
  exit 1
fi
echo "前置检查: 服务在 8080 运行 ✓"
echo

declare -a NAMES=(
  "B-1 农历节日短名解析期拒绝 (blocker)|test-b1.sh"
  "B-2 TOTP 种子加密落库 (blocker)|test-b2.sh"
  "H-1 STRICT_SECRETS 弱密钥拒绝启动|test-h1.sh"
  "H-2 SMTP 认证不得明文降级|test-h2.sh"
  "H-5 备份导入保真模板 enabled|test-h5.sh"
  "M-2 + M-12 CLAIM_TTL 钳制 / CHANGELOG|test-m2-m12.sh"
)

declare -a RESULTS=()
FAILED=0

for item in "${NAMES[@]}"; do
  NAME="${item%%|*}"
  SCRIPT="${item##*|}"
  echo
  echo "############################################################"
  echo "#  $NAME"
  echo "############################################################"
  if bash "$DIR/$SCRIPT"; then
    RESULTS+=("PASS  $NAME")
  else
    RESULTS+=("FAIL  $NAME")
    FAILED=$((FAILED+1))
  fi
  echo
done

echo "############################################################"
echo "#  汇总"
echo "############################################################"
for r in "${RESULTS[@]}"; do echo "  $r"; done
echo
if [ "$FAILED" -eq 0 ]; then
  echo "全部通过（7/7 条目覆盖）"
  exit 0
else
  echo "$FAILED 项失败"
  exit 1
fi
