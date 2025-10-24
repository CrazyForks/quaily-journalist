package worker

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"time"

	"quaily-journalist/internal/hackernews"
	"quaily-journalist/internal/model"
	"quaily-journalist/internal/storage"
)

// HNCollector polls Hacker News story lists, scores items, and stores them into period ZSETs.
type HNCollector struct {
	Client       *hackernews.Client
	Store        *storage.RedisStore
	Lists        []string // e.g., top,new,best,ask,show,job
	Interval     time.Duration
	LimitPerList int // how many IDs to fetch per list
}

func (w *HNCollector) Start(ctx context.Context) error {
	if w.Interval <= 0 {
		w.Interval = 10 * time.Minute
	}
	if w.LimitPerList <= 0 {
		w.LimitPerList = 10
	}

	// initial run
	w.runOnce(ctx)

	t := time.NewTicker(w.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			w.runOnce(ctx)
		}
	}
}

func (w *HNCollector) runOnce(ctx context.Context) {
	day := periodKey("daily", time.Now().UTC())
	week := periodKey("weekly", time.Now().UTC())

	lists := w.Lists
	if len(lists) == 0 {
		lists = []string{"top"}
	}
	for _, list := range lists {
		items, err := w.fetchList(ctx, list, w.LimitPerList)
		if err != nil {
			slog.Error("hn-collector: fetch list error", "list", list, "error", err)
			continue
		}
		stored := 0
		for _, it := range items {
			score := hnPopularityScore(it)
			if score <= 0 {
				continue
			}
			if err := w.Store.AddNews(ctx, "hackernews", day, it, score); err != nil {
				slog.Error("hn-collector: store error", "id", it.ID, "error", err)
				continue
			}
			if err := w.Store.AddNews(ctx, "hackernews", week, it, score); err != nil {
				slog.Error("hn-collector: store error", "id", it.ID, "error", err)
				continue
			}
			stored++
		}
		slog.Info("hn-collector: completed for list", "list", list, "stored", stored, "periods", []string{day, week})
	}
}

func (w *HNCollector) fetchList(ctx context.Context, list string, limit int) ([]model.NewsItem, error) {
	switch strings.ToLower(strings.TrimSpace(list)) {
	case "top", "topstories":
		return w.Client.TopStories(ctx, limit)
	case "new", "newstories":
		return w.Client.NewStories(ctx, limit)
	case "best", "beststories":
		return w.Client.BestStories(ctx, limit)
	case "ask", "askstories":
		return w.Client.AskStories(ctx, limit)
	case "show", "showstories":
		return w.Client.ShowStories(ctx, limit)
	case "job", "jobs", "jobstories":
		return w.Client.JobStories(ctx, limit)
	default:
		// unknown list; default to top
		return w.Client.TopStories(ctx, limit)
	}
}

// hnPopularityScore uses HN points (score) and age for time-decayed ranking.
func hnPopularityScore(it model.NewsItem) float64 {
	if it.Points <= 0 {
		return 0
	}
	count := it.Points
	diff := time.Since(it.CreatedAt).Hours()
	if diff < 0 {
		diff = 0
	}
	// Score = (count-1) / (diff+2)^1.8
	score := float64(count-1) / math.Pow(diff+2, 1.8)
	if math.IsNaN(score) || score < 0 {
		score = 0
	}
	return score
}
