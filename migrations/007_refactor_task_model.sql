-- 迁移 007: 重构任务模型，简化步骤记录，删除推送日志表
-- 日期: 2026-04-10

-- 1. 更新 tasks 表，添加关键字段
ALTER TABLE tasks 
  ADD COLUMN model_output_json TEXT COMMENT '模型提取的 JSON' AFTER raw_text,
  ADD COLUMN saved_data_json TEXT COMMENT '保存到数据库的数据 JSON' AFTER model_output_json,
  ADD COLUMN push_response_json TEXT COMMENT '推送接口返回的 JSON' AFTER saved_data_json;

-- 2. 更新 task_steps 表，简化字段
ALTER TABLE task_steps 
  CHANGE COLUMN step step_type VARCHAR(64) NOT NULL COMMENT '步骤类型：parse/save/push',
  CHANGE COLUMN input summary TEXT COMMENT '步骤摘要',
  DROP COLUMN output;

-- 3. 删除 push_logs 表（不再需要）
DROP TABLE IF EXISTS push_logs;
