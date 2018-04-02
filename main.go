package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
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

// Items is my own array struct for sorting
type Items []*gofeed.Item

// Items sort interface
func (it Items) Len() int {
	return len(it)
}

// Items sort interface
func (it Items) Swap(i, j int) {
	it[i], it[j] = it[j], it[i]
}

// Items sort interface
func (it Items) Less(i, j int) bool {
	return it[i].PublishedParsed.Unix()-it[j].PublishedParsed.Unix() < 0
}

// Check ArsTechnica feed
func refreshHandler(w http.ResponseWriter, r *http.Request) {
	// Fetch feed
	ctx := appengine.NewContext(r)
	fp := gofeed.NewParser()
	client := urlfetch.Client(ctx)
	resp, err := client.Get(FEED)
	if err != nil {
		log(w, err.Error())
		return
	}

	// Parse feed
	feed, err := fp.Parse(resp.Body)
	if err != nil {
		log(w, err.Error())
		return
	}

	// Get unix timestamp for last article sent
	key := datastore.NewKey(ctx, "Latest", "latest", 0, nil)
	var latest Latest
	datastore.Get(ctx, key, &latest)

	// Filter new items
	items := make(Items, 0)
	for _, item := range feed.Items {
		if item.PublishedParsed.Unix()-latest.Latest > 0 {
			items = append(items, item)
		}
	}

	// Send new items to Telegram bot
	if len(items) > 0 {
		sort.Sort(items)
		for _, item := range items {
			// Store latest
			latest = Latest{Latest: item.PublishedParsed.Unix()}
			if _, err := datastore.Put(ctx, key, &latest); err != nil {
				log(w, err.Error())
				return
			}
			msg := fmt.Sprintf("%s\n%s\n%s", item.Title, item.Published, item.Link)
			sendMessage(ctx, msg)
		}
	}
	log(w, "OK")
}

// Start app
func main() {
	http.HandleFunc("/refresh", refreshHandler)
	appengine.Main()
}

// Send Telegram message
func sendMessage(ctx context.Context, msg string) bool {
	json := []byte(`{"chat_id":"` + CHAT + `", "text":"` + msg + `"}`)
	body := bytes.NewBuffer(json)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", TOKEN)
	client := urlfetch.Client(ctx)
	_, err := client.Post(url, "application/json", body)
	return err == nil
}

// Helper logging to response
func log(w http.ResponseWriter, val interface{}) {
	w.Write([]byte(fmt.Sprintf("%v\n\n", val)))
}
