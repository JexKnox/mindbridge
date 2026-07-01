-- 知识库元信息表
-- 每个 KB 对应 Qdrant 中的一个 collection(kb_{id}),实现库间隔离。
-- embedding 维度与模型一旦确定不可随意更改(维度变更需重建 collection)。
CREATE TABLE IF NOT EXISTS kb (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT         NOT NULL,
    collection      TEXT         NOT NULL UNIQUE,   -- 形如 kb_{id}
    embedding_model TEXT         NOT NULL,
    embedding_dim   INTEGER      NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_kb_name ON kb (name);
