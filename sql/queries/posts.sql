-- name: CreatePost :one
INSERT INTO posts (id, created_at, updated_at, title, url, description, published_at, feed_id)
VALUES($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPostsByUser :many
SELECT title,posts.url,posts.description, published_at, posts.created_at, posts.updated_at from users
inner join feeds on users.id = feeds.user_id
inner join posts on feeds.id = posts.feed_id
where users.id = $1
order by published_at desc
limit $2;
