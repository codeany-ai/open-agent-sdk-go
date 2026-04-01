package costtracker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/shipany-ai/open-agent-sdk-go/types"
)

// ModelUsage tracks usage for a specific model.
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	CostUSD                  float64 `json:"costUSD"`
}

// Tracker tracks cumulative costs and token usage.
type Tracker struct {
	mu           sync.RWMutex
	totalCostUSD float64
	modelUsage   map[string]*ModelUsage
	sessionID    string
}

// NewTracker creates a new cost tracker.
func NewTracker(sessionID string) *Tracker {
	return &Tracker{
		modelUsage: make(map[string]*ModelUsage),
		sessionID:  sessionID,
	}
}

// pricing per million tokens (approximate)
var modelPricing = map[string]struct {
	inputPerM, outputPerM, cacheReadPerM, cacheWritePerM float64
}{
	"claude-sonnet-4-6":   {3.0, 15.0, 0.3, 3.75},
	"claude-opus-4-6":     {15.0, 75.0, 1.5, 18.75},
	"claude-haiku-4-5":    {0.8, 4.0, 0.08, 1.0},
	"claude-sonnet-4-5":   {3.0, 15.0, 0.3, 3.75},
}

// AddUsage records token usage and cost for a model.
func (t *Tracker) AddUsage(model string, usage *types.Usage) float64 {
	if usage == nil {
		return t.TotalCost()
	}

	cost := calculateCost(model, usage)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.totalCostUSD += cost

	mu, ok := t.modelUsage[model]
	if !ok {
		mu = &ModelUsage{}
		t.modelUsage[model] = mu
	}

	mu.InputTokens += usage.InputTokens
	mu.OutputTokens += usage.OutputTokens
	mu.CacheReadInputTokens += usage.CacheReadInputTokens
	mu.CacheCreationInputTokens += usage.CacheCreationInputTokens
	mu.CostUSD += cost

	return t.totalCostUSD
}

// TotalCost returns the total accumulated cost in USD.
func (t *Tracker) TotalCost() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.totalCostUSD
}

// GetModelUsage returns usage for a specific model.
func (t *Tracker) GetModelUsage(model string) *ModelUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if mu, ok := t.modelUsage[model]; ok {
		cp := *mu
		return &cp
	}
	return nil
}

// AllModelUsage returns usage for all models.
func (t *Tracker) AllModelUsage() map[string]*ModelUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]*ModelUsage, len(t.modelUsage))
	for k, v := range t.modelUsage {
		cp := *v
		result[k] = &cp
	}
	return result
}

// TotalTokens returns total input and output tokens across all models.
func (t *Tracker) TotalTokens() (input, output int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, mu := range t.modelUsage {
		input += mu.InputTokens
		output += mu.OutputTokens
	}
	return
}

// FormatCost returns a human-readable cost string.
func (t *Tracker) FormatCost() string {
	cost := t.TotalCost()
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

// Save persists cost state to disk.
func (t *Tracker) Save(configDir string) error {
	t.mu.RLock()
	state := map[string]interface{}{
		"sessionId":  t.sessionID,
		"totalCost":  t.totalCostUSD,
		"modelUsage": t.modelUsage,
	}
	t.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	costFile := filepath.Join(configDir, "cost-state.json")
	return os.WriteFile(costFile, data, 0644)
}

// Restore loads cost state from disk.
func (t *Tracker) Restore(configDir string) error {
	costFile := filepath.Join(configDir, "cost-state.json")
	data, err := os.ReadFile(costFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state struct {
		SessionID  string                  `json:"sessionId"`
		TotalCost  float64                 `json:"totalCost"`
		ModelUsage map[string]*ModelUsage  `json:"modelUsage"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if state.SessionID != t.sessionID {
		return nil // Different session, don't restore
	}

	t.mu.Lock()
	t.totalCostUSD = state.TotalCost
	if state.ModelUsage != nil {
		t.modelUsage = state.ModelUsage
	}
	t.mu.Unlock()

	return nil
}

func calculateCost(model string, usage *types.Usage) float64 {
	pricing, ok := modelPricing[model]
	if !ok {
		// Default to sonnet pricing
		pricing = modelPricing["claude-sonnet-4-6"]
	}

	cost := float64(usage.InputTokens) * pricing.inputPerM / 1_000_000
	cost += float64(usage.OutputTokens) * pricing.outputPerM / 1_000_000
	cost += float64(usage.CacheReadInputTokens) * pricing.cacheReadPerM / 1_000_000
	cost += float64(usage.CacheCreationInputTokens) * pricing.cacheWritePerM / 1_000_000

	return cost
}
