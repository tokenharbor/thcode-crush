package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"charm.land/catwalk/pkg/catwalk"
)

// Config holds the provider configuration needed for model discovery.
type Config struct {
	ID           string
	BaseURL      string
	APIKey       string
	ExtraHeaders map[string]string
	// Existing models from config — IDs present in this list are skipped
	// during discovery (user-specified models win).
	ExistingModels []catwalk.Model
}

// Resolver resolves variable references (e.g. $ENV_VAR) in config values.
type Resolver interface {
	ResolveValue(val string) (string, error)
}

type modelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// DiscoverModels fetches available models from the provider's /models endpoint.
// It uses the provided context for cancellation and timeout; callers should set
// a deadline (e.g. context.WithTimeout) to avoid blocking indefinitely.
// Models whose IDs already appear in cfg.ExistingModels are skipped —
// user-specified models take precedence.
func DiscoverModels(ctx context.Context, cfg Config, resolver Resolver) ([]catwalk.Model, error) {
	baseURL, _ := resolver.ResolveValue(cfg.BaseURL)
	apiKey, _ := resolver.ResolveValue(cfg.APIKey)
	modelsURL := baseURL + "/models"

	req, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("discover models for provider %s: %w", cfg.ID, err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range cfg.ExtraHeaders {
		resolved, err := resolver.ResolveValue(v)
		if err != nil || resolved == "" {
			continue
		}
		req.Header.Set(k, resolved)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover models for provider %s: %w", cfg.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discover models for provider %s: %s", cfg.ID, resp.Status)
	}

	var modelsResp modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("discover models for provider %s: %w", cfg.ID, err)
	}

	// Build set of existing model IDs to skip.
	existing := make(map[string]struct{}, len(cfg.ExistingModels))
	for _, m := range cfg.ExistingModels {
		existing[m.ID] = struct{}{}
	}

	// Start with user-specified models.
	result := make([]catwalk.Model, len(cfg.ExistingModels))
	copy(result, cfg.ExistingModels)

	// Append discovered models not already in the list.
	for _, e := range modelsResp.Data {
		if _, ok := existing[e.ID]; ok {
			continue
		}
		result = append(result, catwalk.Model{
			ID:   e.ID,
			Name: e.ID,
		})
	}

	return result, nil
}
