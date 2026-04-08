-- 任务主表：每次用户主动发消息触发一个任务
CREATE TABLE IF NOT EXISTS tasks (
    id               VARCHAR(36)  NOT NULL PRIMARY KEY                COMMENT '任务ID (UUID v4)',
    user_id          VARCHAR(64)  NOT NULL                            COMMENT '用户ID',
    chat_id          VARCHAR(64)  NOT NULL DEFAULT ''                 COMMENT '会话ID（群聊时非空）',
    raw_text         TEXT         NOT NULL                            COMMENT '用户原始输入文本',
    status           VARCHAR(16)  NOT NULL DEFAULT 'pending'          COMMENT 'pending / completed / failed / rejected',
    total_duration_ms INT          DEFAULT NULL                       COMMENT '总耗时(ms)',
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at     DATETIME     DEFAULT NULL,
    INDEX idx_user_id   (user_id),
    INDEX idx_status    (status),
    INDEX idx_created_at(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户任务主表';

-- 任务步骤表：每个处理环节的输入输出与耗时
CREATE TABLE IF NOT EXISTS task_steps (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    task_id      VARCHAR(36)  NOT NULL                                COMMENT '关联任务ID',
    step         VARCHAR(64)  NOT NULL                                COMMENT '步骤名: intent_check / llm_parse / validate / db_save',
    status       VARCHAR(16)  NOT NULL DEFAULT 'success'             COMMENT 'success / failed / skipped',
    input        TEXT         DEFAULT NULL                            COMMENT '步骤输入（文本或JSON）',
    output       TEXT         DEFAULT NULL                            COMMENT '步骤输出（文本或JSON）',
    duration_ms  INT          DEFAULT NULL                            COMMENT '步骤耗时(ms)',
    error        TEXT         DEFAULT NULL                            COMMENT '错误信息（失败时填写）',
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_id (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务步骤记录表';
