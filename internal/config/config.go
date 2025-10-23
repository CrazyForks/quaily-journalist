package config

// AppConfig holds application-level settings.
type AppConfig struct {
	LogLevel string `mapstructure:"log_level"`
}

// RedisConfig holds redis connection settings.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// V2EXConfig controls the V2EX data source.
type V2EXConfig struct {
	Token         string `mapstructure:"token"`
	BaseURL       string `mapstructure:"base_url"`
	FetchInterval string `mapstructure:"fetch_interval"` // duration string, e.g., "5m"
}

// HackerNewsConfig controls the Hacker News data source.
type HackerNewsConfig struct {
	BaseAPI       string `mapstructure:"base_api"`       // API base, defaults to https://hacker-news.firebaseio.com/v0
	FetchInterval string `mapstructure:"fetch_interval"` // duration string, e.g., "10m"
}

// DataSources groups available collectors.
type DataSources struct {
	V2EX V2EXConfig       `mapstructure:"v2ex"`
	HN   HackerNewsConfig `mapstructure:"hackernews"`
}

// OpenAIConfig holds OpenAI settings.
type OpenAIConfig struct {
	APIKey  string `mapstructure:"api_key"`
	Model   string `mapstructure:"model"`
	BaseURL string `mapstructure:"base_url"`
}

// NewsletterConfig controls publication logic.
type NewslettersConfig struct {
	Frequency string          `mapstructure:"frequency"` // default frequency
	TopN      int             `mapstructure:"top_n"`     // default top N
	MinItems  int             `mapstructure:"min_items"` // default min items
	OutputDir string          `mapstructure:"output_dir"`
	Channels  []ChannelConfig `mapstructure:"channels"`
}

// ChannelTemplate groups text fields for rendering.
type ChannelTemplate struct {
	Title      string `mapstructure:"title"`
	Preface    string `mapstructure:"preface"`
	Postscript string `mapstructure:"postscript"`
}

// ChannelConfig defines a newsletter channel bound to a single source.
type ChannelConfig struct {
	Name             string          `mapstructure:"name"`      // e.g., v2ex_daily_digest
	Source           string          `mapstructure:"source"`    // e.g., v2ex
	Frequency        string          `mapstructure:"frequency"` // overrides default
	TopN             int             `mapstructure:"top_n"`
	MinItems         int             `mapstructure:"min_items"`
	OutputDir        string          `mapstructure:"output_dir"`         // overrides default
	Nodes            []string        `mapstructure:"nodes"`              // source-specific nodes (e.g., V2EX node names)
	ItemSkipDuration string          `mapstructure:"item_skip_duration"` // e.g., "72h"
	Template         ChannelTemplate `mapstructure:"template"`
	// Legacy fields to maintain backward compatibility; copied into Template in FillDefaults.
	PrefaceLegacy    string `mapstructure:"preface"`
	PostscriptLegacy string `mapstructure:"postscript"`
	Language         string `mapstructure:"language"` // e.g., "English", "中文", affects AI output
}

// Config is the top-level configuration structure.
type Config struct {
	App         AppConfig         `mapstructure:"app"`
	Redis       RedisConfig       `mapstructure:"redis"`
	Sources     DataSources       `mapstructure:"sources"`
	OpenAI      OpenAIConfig      `mapstructure:"openai"`
	Newsletters NewslettersConfig `mapstructure:"newsletters"`
}

// FillDefaults applies default values if not provided.
func (c *Config) FillDefaults() {
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.Redis.Addr == "" {
		c.Redis.Addr = "127.0.0.1:6379"
	}
	if c.Sources.V2EX.BaseURL == "" {
		c.Sources.V2EX.BaseURL = "https://www.v2ex.com"
	}
	if c.Sources.V2EX.FetchInterval == "" {
		c.Sources.V2EX.FetchInterval = "10m"
	}
	if c.Sources.HN.BaseAPI == "" {
		c.Sources.HN.BaseAPI = "https://hacker-news.firebaseio.com/v0"
	}
	if c.Sources.HN.FetchInterval == "" {
		c.Sources.HN.FetchInterval = "10m"
	}
	if c.Newsletters.Frequency == "" {
		c.Newsletters.Frequency = "daily"
	}
	if c.Newsletters.TopN == 0 {
		c.Newsletters.TopN = 20
	}
	if c.Newsletters.MinItems == 0 {
		c.Newsletters.MinItems = 5
	}
	if c.Newsletters.OutputDir == "" {
		c.Newsletters.OutputDir = "./out"
	}
	// Fill channel defaults
	for i := range c.Newsletters.Channels {
		ch := &c.Newsletters.Channels[i]
		if ch.Frequency == "" {
			ch.Frequency = c.Newsletters.Frequency
		}
		if ch.TopN == 0 {
			ch.TopN = c.Newsletters.TopN
		}
		if ch.MinItems == 0 {
			ch.MinItems = c.Newsletters.MinItems
		}
		if ch.OutputDir == "" {
			ch.OutputDir = c.Newsletters.OutputDir
		}
		if ch.ItemSkipDuration == "" {
			ch.ItemSkipDuration = "72h"
		}
		if ch.Language == "" {
			ch.Language = "English"
		}
		// Backward-compat: map legacy fields into template if present.
		if ch.Template.Preface == "" && ch.PrefaceLegacy != "" {
			ch.Template.Preface = ch.PrefaceLegacy
		}
		if ch.Template.Postscript == "" && ch.PostscriptLegacy != "" {
			ch.Template.Postscript = ch.PostscriptLegacy
		}
	}
}
