-- 删除墓碑与时间格式规范化。
-- 时间列统一为固定宽度 UTC：YYYY-MM-DDTHH:MM:SS.sssZ。
-- 固定宽度保证 TEXT 的 BINARY collation 排序等价于时间排序。

ALTER TABLE items ADD COLUMN deleted_at TEXT;

UPDATE items
SET start_at = strftime('%Y-%m-%dT%H:%M:%fZ', start_at)
WHERE start_at IS NOT NULL AND start_at <> '';

UPDATE items
SET end_at = strftime('%Y-%m-%dT%H:%M:%fZ', end_at)
WHERE end_at IS NOT NULL AND end_at <> '';

UPDATE items
SET completed_at = strftime('%Y-%m-%dT%H:%M:%fZ', completed_at)
WHERE completed_at IS NOT NULL AND completed_at <> '';

UPDATE items
SET created_at = strftime('%Y-%m-%dT%H:%M:%fZ', created_at)
WHERE created_at IS NOT NULL AND created_at <> '';

UPDATE items
SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', updated_at)
WHERE updated_at IS NOT NULL AND updated_at <> '';

CREATE INDEX IF NOT EXISTS idx_items_deleted_at ON items(deleted_at);
