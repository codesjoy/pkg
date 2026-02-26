DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name character varying(255) NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at bigint NOT NULL,
    "update" bigint NOT NULL,
    deleted_at bigint NULL
);

CREATE INDEX idx_users_created_id ON users (created_at, id);
CREATE INDEX idx_users_deleted_at ON users (deleted_at);
