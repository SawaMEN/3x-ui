package systemupdate

// CleanupResult reports the effects of an explicitly requested cache cleanup.
// FreedBytes is an approximate difference in available filesystem space.
type CleanupResult struct {
	FreedBytes   int64    `json:"freedBytes"`
	RemovedFiles int      `json:"removedFiles"`
	Actions      []string `json:"actions"`
	Warnings     []string `json:"warnings"`
}
