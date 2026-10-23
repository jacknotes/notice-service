#!/usr/bin/env bash
# H-5 验收：备份导入必须保真模板 enabled
# 用法：wsl -e bash /mnt/f/project/notice-service/docs/fixes/test-h5.sh
set -uo pipefail

BASE="http://127.0.0.1:8080"
cd /mnt/f/project/notice-service || exit 1

login() {
  curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin123"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])'
}
TOK=$(login)
[ -z "$TOK" ] && { echo "!! 登录失败"; exit 1; }

echo "=============================================="
echo " 0) 当前库内状态（问题现场）"
echo "=============================================="
mysql -unotice -pnotice123 -h127.0.0.1 -e "
SELECT enabled, COUNT(*) AS cnt FROM notice_service.templates WHERE deleted_at IS NULL GROUP BY enabled;
SELECT COUNT(*) AS tasks_pointing_to_disabled_templates
FROM notice_service.tasks t
JOIN notice_service.templates tpl ON tpl.id=t.template_id
WHERE t.deleted_at IS NULL AND tpl.enabled=0;" 2>/dev/null

echo
echo "=============================================="
echo " 1) 导出备份"
echo "=============================================="
curl -s -H "Authorization: Bearer $TOK" "$BASE/api/export" -o /tmp/bk.json -w 'export HTTP=%{http_code} bytes=%{size_download}\n'

echo
echo "=============================================="
echo " 2) 构造「1 启用 + 1 停用」的最小 bundle"
echo "=============================================="
python3 - <<'PY'
import json, time
b = json.load(open("/tmp/bk.json"))
tp = b["templates"]
on = [t for t in tp if t["enabled"]][0]
off = [t for t in tp if not t["enabled"]][0]
s = str(int(time.time()))
on["name"] = "RVW_ON_" + s
off["name"] = "RVW_OFF_" + s
out = {"version": 1, "exported_at": b["exported_at"], "channels": [],
       "templates": [on, off], "tasks": []}
json.dump(out, open("/tmp/probe.json", "w"), ensure_ascii=False)
print("bundle:", [(t["name"], t["enabled"]) for t in [on, off]])
PY

echo
echo "=============================================="
echo " 3) 导入"
echo "=============================================="
curl -s -X POST "$BASE/api/import" \
  -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' \
  --data-binary @/tmp/probe.json \
  -w '\nimport HTTP=%{http_code}\n'

echo
echo "=============================================="
echo " 4) 读库断言"
echo "=============================================="
mysql -unotice -pnotice123 -h127.0.0.1 -e \
  "SELECT name, enabled FROM notice_service.templates WHERE name LIKE 'RVW_%' ORDER BY name;" 2>/dev/null

echo
echo "--- 断言 ---"
ON_VAL=$(mysql -unotice -pnotice123 -h127.0.0.1 -N -e \
  "SELECT enabled FROM notice_service.templates WHERE name LIKE 'RVW_ON_%' ORDER BY id DESC LIMIT 1;" 2>/dev/null)
OFF_VAL=$(mysql -unotice -pnotice123 -h127.0.0.1 -N -e \
  "SELECT enabled FROM notice_service.templates WHERE name LIKE 'RVW_OFF_%' ORDER BY id DESC LIMIT 1;" 2>/dev/null)

if [ "$ON_VAL" = "1" ] && [ "$OFF_VAL" = "0" ]; then
  echo "PASS: 启用模板导入后仍启用(1)，停用模板仍停用(0)——状态保真"
  echo "      （修复前两个都会是 0，即「状态归零」）"
else
  echo "FAIL: RVW_ON=$ON_VAL RVW_OFF=$OFF_VAL（期望 1 / 0）"
fi

echo
echo "=============================================="
echo " 5) 清理探针"
echo "=============================================="
mysql -unotice -pnotice123 -h127.0.0.1 -e \
  "DELETE FROM notice_service.templates WHERE name LIKE 'RVW_%';" 2>/dev/null
rm -f /tmp/bk.json /tmp/probe.json
mysql -unotice -pnotice123 -h127.0.0.1 -e \
  "SELECT COUNT(*) AS remaining_templates FROM notice_service.templates WHERE deleted_at IS NULL;" 2>/dev/null
