package worker

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"quaily-journalist/internal/model"
	"quaily-journalist/internal/storage"
	"quaily-journalist/internal/v2ex"
)

type V2EXCollector struct {
	Client   *v2ex.Client
	Store    *storage.RedisStore
	Nodes    []string
	Interval time.Duration
}

func (w *V2EXCollector) Start(ctx context.Context) error {
	if w.Interval <= 0 {
		w.Interval = 60 * time.Minute
	}
	t := time.NewTicker(w.Interval)
	defer t.Stop()

	// initial run
	w.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			w.runOnce(ctx)
		}
	}
}

func (w *V2EXCollector) runOnce(ctx context.Context) {
	// Collector writes into both daily and weekly periods for simplicity.
	day := periodKey("daily", time.Now().UTC())
	week := periodKey("weekly", time.Now().UTC())
	for _, node := range w.Nodes {
		items, err := w.Client.TopicsByNode(ctx, node)
		if err != nil {
			slog.Error("run v2ex collector failed.", "node", node, "error", err)
			continue
		}
		for _, it := range items {
			score := popularityScore(it)
			if score <= 0 {
				continue // ignore posts with no replies or low score
			}
			if err := w.Store.AddNews(ctx, "v2ex", day, it, score); err != nil {
				slog.Error("run v2ex collector store error.", "id", it.ID, "error", err)
			}
			if err := w.Store.AddNews(ctx, "v2ex", week, it, score); err != nil {
				slog.Error("run v2ex collector store error.", "id", it.ID, "error", err)
			}
		}
		slog.Info("v2ex collector: completed for node", "node", node, "stored", len(items), "periods", []string{day, week})
	}
}

func popularityScore(it model.NewsItem) float64 {
	// Ignore posts with no replies
	if it.Replies <= 0 {
		return 0
	}
	count := it.Replies // use replies as count
	// hours since published
	diff := time.Since(it.CreatedAt).Hours()
	if diff < 0 {
		diff = 0
	}
	// Hacker News-like score:
	// Score = (count-1) / (diff+2)^1.8
	score := float64(count-1) / math.Pow(diff+2, 1.8)
	if math.IsNaN(score) || score < 0 {
		score = 0
	}
	return score
}

func periodKey(freq string, t time.Time) string {
	utc := t.UTC()
	switch freq {
	case "weekly":
		y, w := utc.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", y, w)
	default: // daily
		return utc.Format("2006-01-02")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
