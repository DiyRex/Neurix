package scraper

import "context"

// Scraper abstracts data acquisition from Ollama.
// Implementations: HTTPScraper (production), MockScraper (tests).
type Scraper interface {
	Scrape(ctx context.Context) (*OllamaData, error)
}
