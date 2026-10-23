#!/usr/bin/env bash
# B-2 验收：TOTP 种子必须加密落库（blocker）
# 用法：wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-b2.sh
set -uo pipefail

BASE="http://127.0.0.1:8080"
cd /mnt/f/project/notice-service || exit 1

login() {
  curl -s -X POST "$BASE/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin123"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])'
}

TOK=$(login)
if [ -z "$TOK" ]; then echo "!! 登录失败，服务没在跑？"; exit 1; fi

echo "=============================================="
echo " 1) 调用 /auth/2fa/setup 生成密钥"
echo "=============================================="
SETUP=$(curl -s -X POST "$BASE/api/auth/2fa/setup" -H "Authorization: Bearer $TOK")
SECRET=$(echo "$SETUP" | python3 -c 'import sys,json; print(json.load(sys.stdin)["secret"])')
echo "secret=$SECRET"

echo
echo "=============================================="
echo " 2) 用 docs/fixes/totp.py 算出 6 位动态码并启用"
echo "=============================================="
CODE=$(python3 docs/fixes/totp.py "$SECRET")
echo "code=$CODE"
curl -s -X POST "$BASE/api/auth/2fa/enable" \
  -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' \
  -d "{\"code\":\"$CODE\"}" \
  -w '\nHTTP=%{http_code}\n'

echo
echo "=============================================="
echo " 3) 直接读库（关键断言）"
echo "=============================================="
mysql -unotice -pnotice123 -h127.0.0.1 -e "
SELECT username,
       totp_enabled,
       totp_secret IS NULL          AS plain_is_null,
       CHAR_LENGTH(totp_secret_enc) AS enc_len
FROM notice_service.users WHERE username='admin';" 2>/dev/null

echo
echo "--- 断言 ---"
PLAIN_NULL=$(mysql -unotice -pnotice123 -h127.0.0.1 -N -e \
  "SELECT totp_secret IS NULL FROM notice_service.users WHERE username='admin';" 2>/dev/null)
ENC_LEN=$(mysql -unotice -pnotice123 -h127.0.0.1 -N -e \
  "SELECT COALESCE(CHAR_LENGTH(totp_secret_enc),0) FROM notice_service.users WHERE username='admin';" 2>/dev/null)

if [ "$PLAIN_NULL" = "1" ] && [ "$ENC_LEN" -gt 0 ] 2>/dev/null; then
  echo "PASS: 明文列为 NULL，密文列长度=$ENC_LEN（B-2 闭环）"
else
  echo "FAIL: plain_is_null=$PLAIN_NULL enc_len=$ENC_LEN（明文落库 = blocker 未修）"
fi

echo
echo "=============================================="
echo " 4) 收尾：关闭 2FA，验证两列都被清空"
echo "=============================================="
# 用动态码关闭
CODE2=$(python3 docs/fixes/totp.py "$SECRET")
curl -s -X POST "$BASE/api/auth/2fa/disable" \
  -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' \
  -d "{\"code\":\"$CODE2\"}" \
  -w '\nHTTP=%{http_code}\n'

mysql -unotice -pnotice123 -h127.0.0.1 -e "
SELECT username, totp_enabled,
       totp_secret IS NULL AS p, totp_secret_enc IS NULL AS e,
       totp_last_counter
FROM notice_service.users WHERE username='admin';" 2>/dev/null

echo
echo "（若上面的 disable 因动态码时间步已翻页而失败，用这条兜底恢复：）"
echo "  mysql -unotice -pnotice123 -h127.0.0.1 -e \"UPDATE notice_service.users SET totp_secret=NULL,totp_secret_enc=NULL,totp_enabled=0,totp_recovery_codes=NULL,totp_last_counter=0 WHERE username='admin';\""
