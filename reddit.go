package main

type (
	Subreddit struct {
		Kind string
		Data *SubredditData
		Code int
	}

	SubredditData struct {
		Modhash  string
		Children []RedditPost
		After    string
		Before   string
	}

	RedditPost struct {
		Kind string
		Data PostData
	}

	PostData struct {
		Domain      string
		Id          string
		NumComments int `json:"num_comments"`
		Score       int
		Over18      bool `json:"over_18"`
		Url         string
		Media       *Media
		Title       string
	}

	Media struct {
		Type   string
		Oembed EmbedMedia
	}

	EmbedMedia struct {
		ThumbnailUrl string `json:"thumbnail_url"`
	}
)
