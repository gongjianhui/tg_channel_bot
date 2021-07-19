package fetchers

import (
	"errors"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"github.com/asdine/storm"
	"github.com/patrickmn/go-cache"
	"net/url"
	"time"
)

type TwitterFetcher struct {
	BaseFetcher
	api               *anaconda.TwitterApi
	AccessToken       string `json:"access_token"`
	AccessTokenSecret string `json:"access_token_secret"`
	ConsumerKey       string `json:"consumer_key"`
	ConsumerSecret    string `json:"consumer_secret"`
	cache             *cache.Cache
	channel_id        string
}

const (
	MaxTweetCount = "20"
)

func (f *TwitterFetcher) Init(db *storm.DB, channel_id string) (err error) {
	f.DB = db.From("twitter")
	f.api = anaconda.NewTwitterApiWithCredentials(f.AccessToken, f.AccessTokenSecret, f.ConsumerKey, f.ConsumerSecret)
	f.cache = cache.New(cacheExp*time.Hour, cachePurge*time.Hour)
	f.channel_id = channel_id
	return
}

func (f *TwitterFetcher) getUserTimeline(user string, time int64) ([]ReplyMessage, error) {
	v := url.Values{}
	v.Set("count", MaxTweetCount)
	v.Set("screen_name", user)
	results, err := f.api.GetUserTimeline(v)
	if err != nil {
		return []ReplyMessage{}, err
	}
	ret := make([]ReplyMessage, 0, len(results))
	for _, tweet := range results {
		t, err := tweet.CreatedAtTime()
		if err != nil {
			continue
		}
		tweet_time := t.Unix()
		if tweet_time < time {
			break
		}
		if len(tweet.InReplyToStatusIdStr) > 0 {
			continue
		}
		if tweet.RetweetedStatus != nil {
			continue
		}
		var msgid string
		msgid = tweet.QuotedStatusIdStr
		if msgid == "" {
			msgid = tweet.IdStr
		}
		msgid = fmt.Sprintf("%s@%s", f.channel_id, msgid)
		_, found := f.cache.Get(msgid)
		f.cache.Set(msgid, true, cache.DefaultExpiration)
		if found {
			continue
		}

		var tweetlink string
		tweetlink += "https://twitter.com/"
		tweetlink += tweet.User.ScreenName
		tweetlink += "/status/"
		tweetlink += tweet.IdStr
		tweetlink += "/"

		resources := make([]Resource, 0, len(tweet.ExtendedEntities.Media))
		ret = append(ret, ReplyMessage{resources, tweetlink, nil})
	}
	return ret, nil
}

func (f *TwitterFetcher) GetPush(userid string, followings []string) []ReplyMessage {
	var last_update int64
	if err := f.DB.Get("last_update", userid, &last_update); err != nil {
		last_update = 0
	}
	ret := make([]ReplyMessage, 0, 0)
	for _, follow := range followings {
		single, err := f.getUserTimeline(follow, last_update)
		if err == nil {
			ret = append(ret, single...)
		}
	}
	if len(ret) != 0 {
		f.DB.Set("last_update", userid, time.Now().Unix())
	}
	return ret
}

func (f *TwitterFetcher) GoBack(userid string, back int64) error {
	now := time.Now().Unix()
	if back > now {
		return errors.New("Back too long!")
	}
	return f.DB.Set("last_update", userid, now-back)
}
