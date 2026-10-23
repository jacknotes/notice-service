#!/usr/bin/env bash
# M-2 + M-12 验收：CLAIM_TTL 钳制不崩进程 / CHANGELOG 非空
# 用法：wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-m2-m12.sh
set -uo pipefail

BIN=/tmp/notice-server
REPO=/mnt/f/project/notice-service

echo "##############################################"
echo "# M-2  QUEUE_CLAIM_TTL 非法值不得崩进程"
echo "##############################################"

echo
echo "=============================================="
echo " 1) QUEUE_CLAIM_TTL=0（修复前 time.NewTicker(0) panic 崩进程）"
echo "=============================================="
QUEUE_CLAIM_TTL=0 QUEUE_WORKERS=0 DB_PASSWORD=notice123 PORT=8082 timeout 6 "$BIN" > /tmp/m2-0.log 2>&1
grep -E '警告|listening|panic' /tmp/m2-0.log | head -8

echo
echo "--- 断言 ---"
if grep -q 'listening' /tmp/m2-0.log && ! grep -qi 'panic' /tmp/m2-0.log; then
  echo "PASS: 未 panic，正常监听（钳制生效）"
else
  echo "FAIL: 出现 panic 或未启动"
fi

echo
echo "=============================================="
echo " 2) QUEUE_CLAIM_TTL=-5（负数）"
echo "=============================================="
QUEUE_CLAIM_TTL=-5 QUEUE_WORKERS=0 DB_PASSWORD=notice123 PORT=8082 timeout 6 "$BIN" > /tmp/m2-neg.log 2>&1
grep -E 'QUEUE_CLAIM_TTL|listening|panic' /tmp/m2-neg.log | head -5

echo
echo "--- 断言 ---"
if grep -q '已钳制为 30' /tmp/m2-neg.log && ! grep -qi 'panic' /tmp/m2-neg.log; then
  echo "PASS: 负数被钳制到 30s"
else
  echo "FAIL: 未钳制或 panic"
fi

echo
echo "=============================================="
echo " 3) 合法值 120/4 不应被改写"
echo "=============================================="
QUEUE_CLAIM_TTL=120 QUEUE_WORKERS=4 DB_PASSWORD=notice123 PORT=8082 timeout 5 "$BIN" > /tmp/m2-ok.log 2>&1
grep -E 'QUEUE_|listening' /tmp/m2-ok.log | head -5

echo
echo "--- 断言 ---"
if ! grep -q 'QUEUE_CLAIM_TTL=120 超过\|QUEUE_CLAIM_TTL=120 低于' /tmp/m2-ok.log; then
  echo "PASS: 合法值未被改写"
else
  echo "FAIL: 合法值被误钳制"
fi

rm -f /tmp/m2-0.log /tmp/m2-neg.log /tmp/m2-ok.log

echo
echo "##############################################"
echo "# M-12 CHANGELOG [Unreleased] 非空"
echo "##############################################"
cd "$REPO" || exit 1

echo
echo "--- [Unreleased] 到 [2.0.0] 之间的行数 ---"
LINES=$(awk '/^## \[Unreleased\]/{f=1;next} /^## \[2\.0\.0\]/{f=0} f' CHANGELOG.md | wc -l)
echo "$LINES"

echo "--- 小节标题 ---"
awk '/^## \[Unreleased\]/{f=1;next} /^## \[2\.0\.0\]/{f=0} f' CHANGELOG.md | grep '^### '

echo "--- 升级注意条数 ---"
NOTES=$(awk '/^### 升级注意/{f=1;next} /^## \[2\.0\.0\]/{f=0} f' CHANGELOG.md | grep -cE '^[0-9]+\.')
echo "$NOTES"

echo "--- 关键条目 ---"
grep -nE '农历节日名必须用全称|STRICT_SECRETS|登录态语义变化|备份导入' CHANGELOG.md | head -6

echo
echo "--- 断言 ---"
if [ "$LINES" -gt 50 ] && [ "$NOTES" -ge 7 ]; then
  echo "PASS: [Unreleased] 有 $LINES 行，升级注意 $NOTES 条"
else
  echo "FAIL: 行数=$LINES 升级注意=$NOTES"
fi
