-- +goose Up

CREATE TABLE feed_follows(
    id UUID PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE NOT NULL,
    feed_id UUID REFERENCES feeds(id) ON DELETE CASCADE NOT NULL
);

-- +goose Down
DROP TABLE feed_follows;
