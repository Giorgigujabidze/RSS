-- name: CreateFeedFollows :one
INSERT INTO feed_follows (id, feed_id, user_id, created_at, updated_at)
VAlUES($1, $2, $3, $4, $5)
RETURNING *;


-- name: GetFeedFollows :many
SELECT * FROM feed_follows WHERE user_id = $1;

-- name: DeleteFeedFollows :one
DELETE FROM feed_follows
WHERE id = $1
RETURNING *;


-- name: GetAllFeedFollows :many
SELECT * FROM feed_follows;
