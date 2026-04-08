-- 创建报价单表
CREATE TABLE IF NOT EXISTS quote_records (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    quote_id VARCHAR(32) NOT NULL UNIQUE COMMENT '报价单ID',
    session_id VARCHAR(128) NOT NULL COMMENT '会话ID',
    user_id VARCHAR(64) NOT NULL COMMENT '用户ID',
    group_id VARCHAR(64) DEFAULT '' COMMENT '群组ID',
    route_pol VARCHAR(64) DEFAULT '' COMMENT '起运港',
    route_pod VARCHAR(64) DEFAULT '' COMMENT '目的港',
    items JSON COMMENT '报价项',
    surcharges JSON COMMENT '附加费',
    currency VARCHAR(8) DEFAULT 'USD' COMMENT '货币',
    total DECIMAL(15,2) DEFAULT 0 COMMENT '总价',
    remarks TEXT COMMENT '备注',
    valid_until DATETIME COMMENT '有效期',
    version INT DEFAULT 1 COMMENT '版本号',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_session_id (session_id),
    INDEX idx_user_id (user_id),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='报价单记录表';

-- 创建会话历史表
CREATE TABLE IF NOT EXISTS session_history (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    session_id VARCHAR(128) NOT NULL COMMENT '会话ID',
    quote_id VARCHAR(32) NOT NULL COMMENT '报价单ID',
    operation VARCHAR(16) NOT NULL COMMENT '操作类型',
    quote_data JSON COMMENT '报价单快照',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_session_id (session_id),
    INDEX idx_quote_id (quote_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会话历史表';
