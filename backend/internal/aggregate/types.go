package aggregate

// Result contains public search data only, never indexer URLs or API keys.
type Result struct {
	SubjectID int        `json:"subject_id"`
	Items     []Resource `json:"items"`
	Warnings  []string   `json:"warnings"`
}

type Resource struct {
	Title       string   `json:"title"`
	AnimeTitle  string   `json:"anime_title"`
	Group       string   `json:"fansub_group"`
	Episode     string   `json:"episode"`
	Quality     string   `json:"quality"`
	Season      int      `json:"season"`
	Size        int64    `json:"size"`
	Published   string   `json:"published"`
	Magnet      string   `json:"magnet"`
	DetailsURL  string   `json:"details_url"`
	Sources     []string `json:"sources"`
	NeedsReview bool     `json:"needs_review"`
}
