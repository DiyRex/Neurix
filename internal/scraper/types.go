package scraper

// TagsResponse is the JSON body from GET /api/tags.
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

type ModelInfo struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Digest     string `json:"digest"`
	ModifiedAt string `json:"modified_at"`
}

// PSResponse is the JSON body from GET /api/ps.
type PSResponse struct {
	Models []RunningModel `json:"models"`
}

type RunningModel struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SizeVRAM  int64  `json:"size_vram"`
	ExpiresAt string `json:"expires_at"`
}

// VersionResponse is the JSON body from GET /api/version.
type VersionResponse struct {
	Version string `json:"version"`
}

// OllamaData is the aggregated snapshot from a single scrape cycle.
// Populated in parallel; Up=false means Ollama is unreachable.
type OllamaData struct {
	Up      bool
	Version string
	Tags    *TagsResponse
	PS      *PSResponse
}
