-- DROP TABLE users;
CREATE DATABASE connect_example;
CREATE TABLE users
(
    id            SERIAL PRIMARY KEY,
    username      VARCHAR(255) UNIQUE       NOT NULL, -- 关联用户ID
    password_hash VARCHAR(255)              NOT NULL, -- 加密后密码
    salt          VARCHAR(255)              NOT NULL, -- 盐值
    created_at    timestamptz DEFAULT now() NOT NULL, -- Unix时间戳，避免时区问题
    updated_at    timestamptz DEFAULT now() NOT NULL
);
COMMENT
    ON TABLE users IS '用户表';