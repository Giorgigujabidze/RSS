package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"rss/internal/database"
	"rss/internal/worker"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	DB *database.Queries
}

type authedHandler func(http.ResponseWriter, *http.Request, database.User)

func main() {
	godotenv.Load(".env")
	port := os.Getenv("PORT")
	dbUrl := os.Getenv("CONN")
	db, err := sql.Open("postgres", dbUrl)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	if err != nil {
		fmt.Println(err)
		return
	}
	dbQueries := database.New(db)
	apiCfg := apiConfig{dbQueries}
	mux := http.NewServeMux()
	server := http.Server{}

	server.Handler = mux
	server.Addr = "localhost:" + port

	mux.HandleFunc("GET /v1/healthz", readinessHandler)
	mux.HandleFunc("GET /v1/err", errHandler)
	mux.HandleFunc("POST /v1/users", apiCfg.createUserHandler)
	mux.HandleFunc("GET /v1/users", apiCfg.middlewareAuth(userInfoHandler))
	mux.HandleFunc("POST /v1/feeds", apiCfg.middlewareAuth(apiCfg.createFeedsHandler))
	mux.HandleFunc("GET /v1/feeds", apiCfg.feedsHandler)
	mux.HandleFunc("POST /v1/feed_follows", apiCfg.middlewareAuth(apiCfg.feedFollowHandler))
	mux.HandleFunc("DELETE /v1/feed_follows/{feedFollowID}", apiCfg.deleteFeedFollowHandler)
	mux.HandleFunc("GET /v1/feed_follows", apiCfg.middlewareAuth(apiCfg.getAllFeedFollowsHanler))
	mux.HandleFunc("GET /v1/posts/{limit}", apiCfg.middlewareAuth(apiCfg.handlePostsGet))
	go func() {
		for {
			select {
			case <-ticker.C:
				go worker.Worker(dbQueries)
			}
		}
	}()
	fmt.Println("Server Running At: ", server.Addr)
	log.Fatal(server.ListenAndServe())

}

func (cfg *apiConfig) handlePostsGet(w http.ResponseWriter, r *http.Request, user database.User) {
	limit, err := strconv.ParseInt(r.PathValue("limit"), 10, 32)
	limitInt32 := int32(limit)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	if limitInt32 == 0 {
		limitInt32 = 10
	}
	arg := database.GetPostsByUserParams{user.ID, limitInt32}
	data, err := cfg.DB.GetPostsByUser(r.Context(), arg)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	respondWithJson(w, 200, data)
}

func fetchFeedFromUrl(url string) {

}

func (cfg *apiConfig) getAllFeedFollowsHanler(w http.ResponseWriter, r *http.Request, user database.User) {
	data, err := cfg.DB.GetAllFeedFollows(r.Context())
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	respondWithJson(w, 200, data)
}

func (cfg *apiConfig) deleteFeedFollowHandler(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("feedFollowID"))
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	_, err = cfg.DB.DeleteFeedFollows(r.Context(), id)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	w.WriteHeader(200)
}

func (cfg *apiConfig) middlewareAuth(handler authedHandler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api_key := strings.TrimPrefix(r.Header.Get("Authorization"), "ApiKey ")
		if api_key == "" {
			respondWithError(w, 401, "no api_key was provided")
			return
		}
		user, err := cfg.DB.GetUser(r.Context(), api_key)
		if err != nil {
			respondWithError(w, 403, err.Error())
			return
		}
		handler(w, r, user)
	})
}

func (cfg *apiConfig) feedFollowHandler(w http.ResponseWriter, r *http.Request, user database.User) {
	type reqStruct struct {
		FeedID uuid.UUID `json:"feed_id"`
	}
	data := reqStruct{}
	err := decodeJson(r.Body, &data)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	feed_follows, err := cfg.createFeedFollow(r.Body, user, data.FeedID)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	_, err = cfg.DB.CreateFeedFollows(r.Context(), feed_follows)
	if err != nil {
		respondWithError(w, 404, err.Error())
		return
	}
	respondWithJson(w, 200, feed_follows)
}

func (cfg *apiConfig) feedsHandler(w http.ResponseWriter, r *http.Request) {
	feeds, err := cfg.DB.GetFeed(r.Context())
	if err != nil {
		respondWithError(w, 500, err.Error())
	}
	respondWithJson(w, 200, feeds)
}

func (cfg *apiConfig) createFeedsHandler(w http.ResponseWriter, r *http.Request, user database.User) {
	type respStruct struct {
		FeedObj       database.CreateFeedParams        `json:"feed"`
		FeedFollowObj database.CreateFeedFollowsParams `json:"feed_follow"`
	}
	feed, err := cfg.createFeed(r.Body, user)
	if err != nil {
		respondWithError(w, 500, err.Error()+"1")
		return
	}
	_, err = cfg.DB.CreateFeed(r.Context(), feed)
	if err != nil {
		respondWithError(w, 500, err.Error()+"2")
		return
	}
	feed_follows, err := cfg.createFeedFollow(r.Body, user, feed.ID)
	if err != nil {
		respondWithError(w, 500, err.Error()+"3")
		return
	}
	resp := respStruct{feed, feed_follows}
	respondWithJson(w, 200, resp)
}

func (cfg *apiConfig) createFeedFollow(body io.Reader, user database.User, feedID uuid.UUID) (database.CreateFeedFollowsParams, error) {
	feed_follows := database.CreateFeedFollowsParams{}
	feed_follows.ID = uuid.New()
	feed_follows.FeedID = feedID
	feed_follows.UserID = user.ID
	feed_follows.CreatedAt = time.Now()
	feed_follows.UpdatedAt = time.Now()
	return feed_follows, nil
}

func (cfg *apiConfig) createFeed(body io.Reader, user database.User) (database.CreateFeedParams, error) {
	type reqStruct struct {
		Name string `json:"name"`
		Url  string `json:"url"`
	}
	data := reqStruct{}
	err := decodeJson(body, &data)
	if err != nil {
		return database.CreateFeedParams{}, err
	}
	feed := database.CreateFeedParams{}
	feed.ID = uuid.New()
	feed.CreatedAt, feed.UpdatedAt = time.Now(), time.Now()
	feed.Name = data.Name
	feed.Url = data.Url
	feed.UserID = user.ID
	return feed, nil
}

func userInfoHandler(w http.ResponseWriter, r *http.Request, user database.User) {
	respondWithJson(w, 200, user)
}

func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type reqStruct struct {
		Name string `json:"name"`
	}
	user := database.CreateUserParams{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&user)
	if err != nil {
		respondWithError(w, 500, "coouldn't decode json")
		return
	}
	user.ID = uuid.New()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	cfg.DB.CreateUser(r.Context(), user)
	respondWithJson(w, 200, user)
}

func errHandler(w http.ResponseWriter, r *http.Request) {
	respondWithError(w, 500, "Internal Server Error")
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	type respHealth struct {
		Status string `json:"status"`
	}
	respondWithJson(w, 200, respHealth{"ok"})
}

func respondWithJson(w http.ResponseWriter, code int, payload interface{}) error {
	w.WriteHeader(code)
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.Write(data)
	return nil
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	type respErr struct {
		Error string `json:"error"`
	}
	data, err := json.Marshal(respErr{message})
	if err != nil {
		return
	}
	w.WriteHeader(code)
	w.Write(data)
}

func decodeJson(body io.Reader, v any) error {
	decoder := json.NewDecoder(body)
	err := decoder.Decode(v)
	return err
}
