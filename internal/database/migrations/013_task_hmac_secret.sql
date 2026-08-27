-- Webhook HMAC 签名密钥独立化：不再与触发凭据 api_key 复用。
-- 存量任务回填为原 api_key（行为不变），此后可独立轮换互不影响。
ALTER TABLE tasks ADD COLUMN hmac_secret VARCHAR(64) NOT NULL DEFAULT '' AFTER require_signature;
UPDATE tasks SET hmac_secret = api_key WHERE api_key IS NOT NULL AND api_key <> '' AND hmac_secret = '';
