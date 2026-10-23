#!/usr/bin/env bash
# H-2 验收：SMTP 认证不得明文降级
# 用法：wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-h2.sh
set -uo pipefail

BASE="http://127.0.0.1:8080"
REPO=/mnt/f/project/notice-service
cd "$REPO" || exit 1

cleanup() { pkill -f fake_smtp.py 2>/dev/null; }
trap cleanup EXIT

echo "=============================================="
echo " 0) 启动两个假 SMTP 服务器"
echo "    2525 = 宣告 CRAM-MD5 LOGIN PLAIN"
echo "    2526 = 只宣告 LOGIN PLAIN"
echo "=============================================="
python3 docs/fixes/fake_smtp.py 2525 "CRAM-MD5 LOGIN PLAIN" > /tmp/smtp2525.log 2>&1 &
python3 docs/fixes/fake_smtp.py 2526 "LOGIN PLAIN" > /tmp/smtp2526.log 2>&1 &
sleep 2
ss -ltn | grep -E '2525|2526' || { echo "!! 假 SMTP 未启动"; exit 1; }

login() {
  curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin123"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])'
}
TOK=$(login)
[ -z "$TOK" ] && { echo "!! 登录失败"; exit 1; }

test_channel() {
  local port="$1" insecure="$2"
  local cfg="{\"host\":\"127.0.0.1\",\"port\":\"$port\",\"username\":\"u@x.com\",\"password\":\"s3cr3t-pass\",\"from\":\"u@x.com\""
  if [ "$insecure" = "true" ]; then cfg="$cfg,\"allow_insecure\":\"true\""; fi
  cfg="$cfg}"
  curl -s -X POST "$BASE/api/channels/0/test" \
    -H "Authorization: Bearer $TOK" \
    -H 'Content-Type: application/json' \
    -d "{\"type\":\"email\",\"config\":$cfg}" \
    -w '\nHTTP=%{http_code}\n'
}

echo
echo "=============================================="
echo " 1) 非 TLS + 未 allow_insecure → 连接期即拒绝"
echo "=============================================="
test_channel 2525 false
echo "--- 2525 日志（应无 MECH 行）---"
cat /tmp/smtp2525.log

echo
echo "=============================================="
echo " 2) 三机制都宣告 + allow_insecure=true → 必须选 CRAM-MD5"
echo "=============================================="
: > /tmp/smtp2525.log
python3 docs/fixes/fake_smtp.py 2525 "CRAM-MD5 LOGIN PLAIN" > /tmp/smtp2525.log 2>&1 &
sleep 1
test_channel 2525 true
sleep 1
echo "--- 服务器实际收到的认证机制 ---"
cat /tmp/smtp2525.log

echo
echo "--- 断言 ---"
if grep -q 'MECH=CRAM-MD5' /tmp/smtp2525.log && ! grep -q 'LOGIN_PASSWORD_B64' /tmp/smtp2525.log; then
  echo "PASS: 实选 CRAM-MD5，口令未明文上网（只有 HMAC 摘要）"
else
  echo "FAIL: 未选 CRAM-MD5 或口令被明文传输"
fi

echo
echo "=============================================="
echo " 3) 只宣告 LOGIN/PLAIN + 未放行 → 必须拒绝且不认证"
echo "=============================================="
test_channel 2526 false
sleep 1
echo "--- 2526 日志（应只有 LISTEN，无 MECH）---"
cat /tmp/smtp2526.log

echo
echo "--- 断言 ---"
if ! grep -q 'MECH=' /tmp/smtp2526.log; then
  echo "PASS: 从未发起认证（非 TLS 未放行时拒绝明文凭据）"
else
  echo "FAIL: 竟然发起了认证"
fi

echo
echo "=============================================="
echo " 4) 只宣告 LOGIN/PLAIN + allow_insecure=true → 允许但打 warning"
echo "=============================================="
test_channel 2526 true
sleep 1
echo "--- 2526 日志 ---"
cat /tmp/smtp2526.log
echo "--- 服务端 warning ---"
grep '非 TLS 连接使用' /tmp/notice.log | tail -2

echo
echo "--- 断言 ---"
if grep -q 'MECH=LOGIN' /tmp/smtp2526.log && grep -q '非 TLS 连接使用 LOGIN' /tmp/notice.log; then
  echo "PASS: 显式放行后走 LOGIN，且服务端留下 warning（明文可见）"
else
  echo "FAIL: 未走 LOGIN 或缺少 warning"
fi
