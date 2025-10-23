package model

import "time"

// NewsItem represents a single news/topic item from a source.
type NewsItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	NodeName  string    `json:"node_name"`
	Replies   int       `json:"replies"`
	Points    int       `json:"points"`
	CreatedAt time.Time `json:"created_at"`
	Content   string    `json:"content"`
}

// WithScore decorates a news item with a ranking score.
type WithScore struct {
	Item  NewsItem
	Score float64
}
