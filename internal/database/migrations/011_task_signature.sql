-- Webhook 可选 HMAC 签名认证：require_signature=1 时须带 X-Timestamp + X-Signature
ALTER TABLE tasks ADD COLUMN require_signature TINYINT(1) NOT NULL DEFAULT 0 AFTER api_key;
