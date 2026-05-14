-- Reference app schema. Edit this, then `sqlc generate` (or `ix generate`).
CREATE SCHEMA IF NOT EXISTS "public";

CREATE TABLE IF NOT EXISTS widgets (
    id         bigserial PRIMARY KEY,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
