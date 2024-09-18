package worker

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"net/http"
	"rss/internal/database"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type apiCfg struct {
	DB *database.Queries
}

type Args struct {
	Limit  int32
	Offset int32
}

type Rss struct {
	XMLName xml.Name `xml:"rss"`
	Text    string   `xml:",chardata"`
	Version string   `xml:"version,attr"`
	Atom    string   `xml:"atom,attr"`
	Channel struct {
		Text          string `xml:",chardata"`
		Title         string `xml:"title"`
		Link          string `xml:"link"`
		Description   string `xml:"description"`
		Generator     string `xml:"generator"`
		Language      string `xml:"language"`
		LastBuildDate string `xml:"lastBuildDate"`
		Item          []struct {
			Text        string `xml:",chardata"`
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Guid        string `xml:"guid"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

func Worker(dbQueries *database.Queries) {

	args := &Args{Limit: 10, Offset: 0}
	apiCfg := apiCfg{DB: dbQueries}
	feeds := apiCfg.fetchFeedsFromDB(args)
	apiCfg.markFeedsFetched(feeds)

	for _, feed := range feeds {
		fmt.Println(feed.Url)
	}

	c := make(chan Rss)
	e := make(chan error)
	wg := sync.WaitGroup{}
	post := database.CreatePostParams{}
	layout := "Mon, 02 Jan 2006 15:04:05 -0700"

	for i := 0; i < len(feeds); i++ {
		wg.Add(1)
		go worker(feeds[i].Url, c, &wg, e)
	}

	for i := 0; i < len(feeds); i++ {
		select {
		case rss := <-c:
			createPost(&post, rss, layout, feeds[i].ID)
			fmt.Println(post)
			if _, err := apiCfg.DB.CreatePost(context.TODO(), post); err != nil {
				fmt.Println(err)
			}
		case err := <-e:
			fmt.Println("Error received:", err)
		}
	}

	wg.Wait()
}

func createPost(post *database.CreatePostParams, rss Rss, layout string, feedId uuid.UUID) {
	post.ID = uuid.New()
	post.CreatedAt = time.Now()
	post.UpdatedAt = time.Now()
	post.Title = rss.Channel.Title
	post.Url = rss.Channel.Link
	post.Description = sql.NullString{rss.Channel.Description, true}
	if len(rss.Channel.Item) > 0 {
		pubDate, err := time.Parse(layout, rss.Channel.Item[0].PubDate)
		if err == nil {
			post.PublishedAt = sql.NullTime{Time: pubDate, Valid: true}
		}
	}
	post.FeedID = feedId
}

func (cfg *apiCfg) markFeedsFetched(feeds []database.Feed) {
	for _, feed := range feeds {
		cfg.DB.MarkFeedFetched(context.TODO(), feed.ID)
	}
}

func (cfg *apiCfg) fetchFeedsFromDB(args *Args) []database.Feed {
	arg := database.GetNextFeedsToFetchParams{
		Limit:  args.Limit,
		Offset: args.Offset,
	}
	feeds, err := cfg.DB.GetNextFeedsToFetch(context.TODO(), arg)
	if err != nil {
		fmt.Println("Error fetching feeds from DB:", err)
		return nil
	}
	args.Offset += 10
	return feeds
}

func fetchFeedFromUrl(url string) (Rss, error) {
	var data Rss
	resp, err := http.Get(url)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return data, fmt.Errorf("failed to fetch data: status code %d", resp.StatusCode)
	}

	decoder := xml.NewDecoder(resp.Body)
	err = decoder.Decode(&data)
	if err != nil {
		return data, fmt.Errorf("error decoding XML: %v", err)
	}
	return data, nil
}

func worker(url string, c chan Rss, wg *sync.WaitGroup, e chan error) {
	defer wg.Done()

	rss, err := fetchFeedFromUrl(url)
	if err != nil {
		fmt.Println("Error fetching feed:", err)
		e <- err
		return
	}

	c <- rss
}
