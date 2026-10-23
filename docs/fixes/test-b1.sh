#!/usr/bin/env bash
# B-1 验收：农历节日短名必须在解析期被拒绝（blocker）
# 用法（在 Windows PowerShell 里）：wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-b1.sh
set -uo pipefail

BASE="http://127.0.0.1:8080"
REPO="/mnt/f/project/notice-service"

login() {
  curl -s -X POST "$BASE/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin123"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])'
}

TOK=$(login)
if [ -z "$TOK" ]; then echo "!! 登录失败，服务没在跑？"; exit 1; fi

echo "=============================================="
echo " 1) 短名 term 中秋  → 期望 HTTP 400 + 报错列可用值"
echo "=============================================="
curl -s -X POST "$BASE/api/tasks" \
  -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' \
  -d '{"name":"rvw_short","channel_id":3,"template_id":17,"trigger_type":"cron","cron_expr":"@lunar term 中秋 09:00","receivers":["a@x.com"],"enabled":true}' \
  -w '\nHTTP=%{http_code}\n'

echo
echo "=============================================="
echo " 2) 全称 term 中秋节 → 期望 HTTP 200 + next_run_at 有具体日期"
echo "=============================================="
RESP=$(curl -s -X POST "$BASE/api/tasks" \
  -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' \
  -d '{"name":"rvw_full","channel_id":3,"template_id":17,"trigger_type":"cron","cron_expr":"@lunar term 中秋节 09:00","receivers":["a@x.com"],"enabled":true}' \
  -w '\nHTTP=%{http_code}')
echo "$RESP"

# 提取 id 与 next_run_at 做断言
ID=$(echo "$RESP" | head -1 | python3 -c 'import sys,json; print(json.load(sys.stdin).get("id",""))' 2>/dev/null)
NEXT=$(echo "$RESP" | head -1 | python3 -c 'import sys,json; print(json.load(sys.stdin).get("next_run_at"))' 2>/dev/null)

echo
echo "--- 断言 ---"
if [ -n "$ID" ] && [ -n "$NEXT" ] && [ "$NEXT" != "None" ] && [ "$NEXT" != "null" ]; then
  echo "PASS: 全称被接受，next_run_at=$NEXT"
else
  echo "FAIL: 全称未返回 next_run_at（id=$ID next=$NEXT）"
fi

# 清理探针任务
if [ -n "$ID" ]; then
  echo "--- 清理探针任务 id=$ID ---"
  curl -s -X DELETE "$BASE/api/tasks/$ID" -H "Authorization: Bearer $TOK" -w 'HTTP=%{http_code}\n'
fi
