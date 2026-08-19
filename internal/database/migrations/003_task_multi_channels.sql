ALTER TABLE tasks ADD COLUMN channel_ids JSON NULL AFTER channel_id;

-- 存量单渠道任务：回填 channel_ids = [channel_id]，新代码读取时无需特判
UPDATE tasks SET channel_ids = JSON_ARRAY(channel_id) WHERE channel_ids IS NULL;
