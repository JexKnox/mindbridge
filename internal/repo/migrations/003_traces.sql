-- 问答留痕:每次 RAG 问答存一条 trace,是线上抽样评测与用户反馈闭环的地基。
-- contexts 存注入 prompt 的 chunk 文本,供 LLM Judge 评 faithfulness(回答是否被证据支撑)。
CREATE TABLE IF NOT EXISTS chat_traces (
    id            UUID              PRIMARY KEY,
    kb_id         TEXT              NOT NULL,
    query         TEXT              NOT NULL,
    answer        TEXT              NOT NULL DEFAULT '',
    citations     JSONB             NOT NULL DEFAULT '[]',
    contexts      JSONB             NOT NULL DEFAULT '[]',
    top_score     DOUBLE PRECISION  NOT NULL DEFAULT 0,
    refused       BOOLEAN           NOT NULL DEFAULT FALSE,
    duration_ms   BIGINT            NOT NULL DEFAULT 0,
    -- LLM Judge 抽样评分:score 为 faithfulness 0~1;verdict 为一句话判定;
    -- verdict='skipped' 表示未被抽中(不再重复取)。
    judge_score   DOUBLE PRECISION,
    judge_verdict TEXT,
    judged_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ       NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_traces_created ON chat_traces (created_at);
CREATE INDEX IF NOT EXISTS idx_traces_kb ON chat_traces (kb_id);
-- Judge 拉取未评审 trace 用的部分索引。
CREATE INDEX IF NOT EXISTS idx_traces_unjudged ON chat_traces (created_at) WHERE judged_at IS NULL;

-- 用户反馈:不只记布尔值,分类后的反馈才能进入下一轮评测集。
CREATE TABLE IF NOT EXISTS chat_feedback (
    id         UUID         PRIMARY KEY,
    trace_id   UUID         NOT NULL REFERENCES chat_traces(id) ON DELETE CASCADE,
    category   TEXT         NOT NULL,  -- helpful/not_found/outdated/irrelevant/not_actionable/other
    comment    TEXT         NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_feedback_trace ON chat_feedback (trace_id);
CREATE INDEX IF NOT EXISTS idx_feedback_category ON chat_feedback (category);
