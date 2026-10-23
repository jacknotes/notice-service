#!/usr/bin/env bash
# H-1 验收：STRICT_SECRETS=1 对占位密钥必须拒绝启动
# 用法：wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-h1.sh
set -uo pipefail

BIN=/tmp/notice-server
if [ ! -x "$BIN" ]; then echo "!! 缺少 $BIN，先编译：cd /mnt/f/project/notice-service && go build -o /tmp/notice-server ./cmd/server"; exit 1; fi

echo "=============================================="
echo " 1) .env.example 的历史占位值 + STRICT_SECRETS=1 → 必须拒绝启动"
echo "=============================================="
STRICT_SECRETS=1 \
  JWT_SECRET=please_change_me_to_32_bytes_random_hex \
  ENCRYPT_KEY=please_change_me_to_32_bytes_random_hex \
  ADMIN_PASS=admin123 \
  DB_PASSWORD=notice123 \
  PORT=8081 "$BIN" > /tmp/h1-weak.log 2>&1
RC=$?
echo "EXIT=$RC"
echo "--- 日志 ---"
cat /tmp/h1-weak.log

echo
echo "--- 断言 ---"
if [ "$RC" -ne 0 ] && grep -q "拒绝启动" /tmp/h1-weak.log; then
  echo "PASS: 弱密钥被拒绝启动（修复前是 0 告警 + 正常启动）"
else
  echo "FAIL: EXIT=$RC，未出现「拒绝启动」"
fi

echo
echo "=============================================="
echo " 2) 强随机密钥 + STRICT_SECRETS=1 → 必须放行（不误伤）"
echo "=============================================="
STRONG_JWT=$(openssl rand -hex 32)
STRONG_ENC=$(openssl rand -hex 32)
STRICT_SECRETS=1 \
  JWT_SECRET="$STRONG_JWT" \
  ENCRYPT_KEY="$STRONG_ENC" \
  ADMIN_PASS='Str0ng!Passphrase#2026' \
  DB_PASSWORD=notice123 \
  PORT=8081 timeout 6 "$BIN" > /tmp/h1-strong.log 2>&1
echo "--- 日志 ---"
grep -E "警告|listening" /tmp/h1-strong.log || cat /tmp/h1-strong.log

echo
echo "--- 断言 ---"
if grep -q "listening" /tmp/h1-strong.log && ! grep -qE "公开占位值|低熵|默认/弱值|常见弱口令" /tmp/h1-strong.log; then
  echo "PASS: 强密钥正常启动，无弱密钥告警（不误拒合法强密钥）"
else
  echo "FAIL: 强密钥被误判，或未成功启动"
fi

echo
echo "=============================================="
echo " 3) 仅 ADMIN_PASS=password（其余强）→ 也必须拦"
echo "=============================================="
STRICT_SECRETS=1 \
  JWT_SECRET="$STRONG_JWT" \
  ENCRYPT_KEY="$STRONG_ENC" \
  ADMIN_PASS=password \
  DB_PASSWORD=notice123 \
  PORT=8081 timeout 6 "$BIN" > /tmp/h1-adminpass.log 2>&1
grep -E "ADMIN_PASS|拒绝启动" /tmp/h1-adminpass.log || tail -3 /tmp/h1-adminpass.log

echo
echo "--- 断言 ---"
if grep -q "拒绝启动" /tmp/h1-adminpass.log; then
  echo "PASS: 弱 ADMIN_PASS 单独也会被拦"
else
  echo "FAIL: 弱 ADMIN_PASS 未被拦截"
fi

echo
echo "=============================================="
echo " 4) .env.example 本身：两密钥必须留空"
echo "=============================================="
grep -nE '^(JWT_SECRET|ENCRYPT_KEY)=' /mnt/f/project/notice-service/.env.example

echo
echo "--- 断言 ---"
BLANK=$(grep -cE $'^(JWT_SECRET|ENCRYPT_KEY)=\r?$' /mnt/f/project/notice-service/.env.example)
if [ "$BLANK" -eq 2 ]; then
  echo "PASS: JWT_SECRET / ENCRYPT_KEY 均为空（compose \${VAR:?} 会前置拦截）"
else
  echo "FAIL: 期望 2 个空值，实际 $BLANK"
fi

rm -f /tmp/h1-weak.log /tmp/h1-strong.log /tmp/h1-adminpass.log
