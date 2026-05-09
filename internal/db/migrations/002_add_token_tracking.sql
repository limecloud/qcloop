-- 添加 token 预算和使用追踪字段

-- 为 batch_jobs 表添加 token_budget_per_item 字段
ALTER TABLE batch_jobs ADD COLUMN token_budget_per_item INTEGER DEFAULT 50000;

-- 为 batch_items 表添加 tokens_used 和 time_used_seconds 字段
ALTER TABLE batch_items ADD COLUMN tokens_used INTEGER DEFAULT 0;
ALTER TABLE batch_items ADD COLUMN time_used_seconds INTEGER DEFAULT 0;

-- 为 attempts 表添加 tokens_used 字段
ALTER TABLE attempts ADD COLUMN tokens_used INTEGER DEFAULT 0;

-- 为 qc_rounds 表添加 tokens_used 字段
ALTER TABLE qc_rounds ADD COLUMN tokens_used INTEGER DEFAULT 0;
