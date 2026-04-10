-- 迁移 008: 添加推送请求参数字段
-- 日期: 2026-04-10

-- 1. 更新 tasks 表，添加推送请求参数字段
ALTER TABLE tasks 
  ADD COLUMN push_request_json TEXT COMMENT '推送接口发送的请求参数 JSON' AFTER saved_data_json;
