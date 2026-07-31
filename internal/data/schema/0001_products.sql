-- sqlc 的 schema 目录:这里放建表 DDL,sqlc 靠它推断 queries 里每个字段的类型。
-- 文件按 000N_ 前缀顺序读取,与迁移工具的命名保持一致。
--
-- 这是随模板附带的示例表,对应 internal/biz 里的 Product。
-- co new 默认会连同示例资源一起删掉(--keep-example 可保留)。

CREATE TABLE IF NOT EXISTS products
(
    id             BIGSERIAL PRIMARY KEY,
    spu_code       TEXT           NOT NULL,
    name           TEXT           NOT NULL,
    description    TEXT           NOT NULL DEFAULT '',
    -- 金额一律 NUMERIC,不用 double precision:
    -- 浮点存不下 0.1 这类十进制小数,累加几次就会出现分位误差
    price          NUMERIC(12, 2) NOT NULL,
    status         TEXT           NOT NULL DEFAULT 'draft',
    -- 只存对象存储的相对 Key,域名放配置(见 internal/pkg/minio.go),
    -- 换 CDN 时改一处配置即可,不用刷全表
    main_media_key TEXT           NOT NULL DEFAULT '',
    quantity       BIGINT         NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    -- 软删除:置位而非物理删除,便于审计与恢复
    deleted_at     TIMESTAMPTZ
);

-- spu_code 是业务主键,必须唯一;带 deleted_at IS NULL 的部分索引,
-- 这样同一个 spu_code 被软删后还能重新建
CREATE UNIQUE INDEX IF NOT EXISTS products_spu_code_key
    ON products (spu_code) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS products_status_created_at_idx
    ON products (status, created_at DESC) WHERE deleted_at IS NULL;
