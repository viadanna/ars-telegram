package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"

	"github.com/mmcdole/gofeed"
)

// Configuration constants
const (
	FEED  = "https://arstechnica.com/feed/"
	CHAT  = "YOUR_CHAT_ID"
	TOKEN = "YOUR_TELEGRAM_BOT_TOKEN"
)

// Latest struct for caching latest
type Latest struct {
	Latest int64
}

// SortableItems is my own array struct for sorting
type SortableItems []*gofeed.Item

// SortableItems sort interface
func (it SortableItems) Len() int {
	return len(it)
}

// SortableItems sort interface
func (it SortableItems) Swap(i, j int) {
	it[i], it[j] = it[j], it[i]
}

// SortableItems sort interface
func (it SortableItems) Less(i, j int) bool {
	return it[i].PublishedParsed.Unix()-it[j].PublishedParsed.Unix() < 0
}

// ArsTechnica new article generator
func fetchArticles(ctx context.Context, latest int64, ch chan *gofeed.Item) {
	// Fetch feed
	fp := gofeed.NewParser()
	client := urlfetch.Client(ctx)
	resp, err := client.Get(FEED)
	if err != nil {
		log.Errorf(ctx, "%v", err)
		return
	}

	// Parse feed
	feed, err := fp.Parse(resp.Body)
	if err != nil {
		log.Errorf(ctx, "%v", err)
		return
	}

	// Use sorting interface
	items := SortableItems(feed.Items)
	sort.Sort(items)

	// Yield new articles
	for _, item := range feed.Items {
		if item.PublishedParsed.Unix()-latest > 0 {
			ch <- item
		}
	}

	// Finish
	close(ch)
}

// Check ArsTechnica feed
func refreshHandler(w http.ResponseWriter, r *http.Request) {
	// Create context
	ctx := appengine.NewContext(r)

	// Get unix timestamp for last article sent
	key := datastore.NewKey(ctx, "Latest", "latest", 0, nil)
	latest := Latest{Latest: 0}
	datastore.Get(ctx, key, &latest)

	// Fetch new articles using generator
	ch := make(chan *gofeed.Item)
	go fetchArticles(ctx, latest.Latest, ch)

	// Control channel for sending messages async
	resultChannel := make(chan bool)
	sending := 0

	for item := range ch {
		// Send new article to Telegram bot
		msg := fmt.Sprintf("%s\n%s\n%s", item.Title, item.Published, item.Link)
		go sendMessage(ctx, msg, resultChannel)
		sending++

		// Store latest
		if item.PublishedParsed.Unix() > latest.Latest {
			latest = Latest{Latest: item.PublishedParsed.Unix()}
		}
	}

	// Store last timestamp
	if _, err := datastore.Put(ctx, key, &latest); err != nil {
		response(w, err.Error())
		return
	}

	// Wait all messages sent
	for i := sending; i > 0; i-- {
		<-resultChannel
	}
	response(w, "OK")
}

// Start app
func main() {
	http.HandleFunc("/refresh", refreshHandler)
	appengine.Main()
}

// Send Telegram message
func sendMessage(ctx context.Context, msg string, ch chan bool) {
	json := []byte(`{"chat_id":"` + CHAT + `", "text":"` + msg + `"}`)
	body := bytes.NewBuffer(json)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", TOKEN)
	client := urlfetch.Client(ctx)
	_, err := client.Post(url, "application/json", body)
	ch <- err == nil
}

// Helper logging to response
func response(w http.ResponseWriter, val interface{}) {
	w.Write([]byte(fmt.Sprintf("%v\n\n", val)))
}
