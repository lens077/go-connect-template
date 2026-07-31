-- sqlc 的 queries 目录:每条查询上方的 -- name: 注释决定生成的方法名与返回形态。
--   :one   返回单行,无结果时返回 pgx.ErrNoRows
--   :many  返回多行
--   :exec  不返回行
--   :execrows 返回受影响行数(用于判断 UPDATE/DELETE 是否命中)
--
-- 这是随模板附带的示例,对应 schema/0001_products.sql。
-- co new 默认会连同示例资源一起删掉(--keep-example 可保留)。

-- name: GetProduct :one
SELECT *
FROM products
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetProductBySpuCode :one
SELECT *
FROM products
WHERE spu_code = $1
  AND deleted_at IS NULL;

-- name: ListProducts :many
-- 用 keyset 分页而不是 OFFSET:OFFSET 需要扫过并丢弃前面所有行,
-- 翻到深页时会线性变慢。这里以 (created_at, id) 为游标。
SELECT *
FROM products
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::TEXT IS NULL OR status = sqlc.narg('status')::TEXT)
  AND (sqlc.narg('cursor_created_at')::TIMESTAMPTZ IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::TIMESTAMPTZ, sqlc.narg('cursor_id')::BIGINT))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_size');

-- name: CountProducts :one
SELECT count(*)
FROM products
WHERE deleted_at IS NULL
  AND (sqlc.narg('status')::TEXT IS NULL OR status = sqlc.narg('status')::TEXT);

-- name: CreateProduct :one
INSERT INTO products (spu_code, name, description, price, status, main_media_key, quantity)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateProduct :one
-- COALESCE + narg 实现部分更新:传 NULL 的字段保持原值,
-- 免得为每种字段组合各写一条 UPDATE
UPDATE products
SET name           = COALESCE(sqlc.narg('name'), name),
    description    = COALESCE(sqlc.narg('description'), description),
    price          = COALESCE(sqlc.narg('price'), price),
    status         = COALESCE(sqlc.narg('status'), status),
    main_media_key = COALESCE(sqlc.narg('main_media_key'), main_media_key),
    quantity       = COALESCE(sqlc.narg('quantity'), quantity),
    updated_at     = now()
WHERE id = sqlc.arg('id')
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteProduct :execrows
-- 返回 execrows 而非 exec:调用方据此区分「删掉了」和「本来就不存在」
UPDATE products
SET deleted_at = now(),
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL;
