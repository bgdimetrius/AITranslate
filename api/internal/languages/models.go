package languages

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Provider identifies a translation backend in the public language catalog.
type Provider string

const (
	ProviderChatGPT Provider = "chatgpt"
	ProviderDeepL   Provider = "deepl"
)

// Capabilities is the public, provider-specific language description returned
// by the API. Source and target entries retain the spelling expected by the
// provider while matching incoming values case-insensitively.
type Capabilities struct {
	Source     []string `json:"source"`
	Target     []string `json:"target"`
	AutoDetect bool     `json:"auto_detect"`
}

type providerLanguages struct {
	source       map[string]string
	target       map[string]string
	capabilities Capabilities
}

// Catalog is an immutable registry of the languages supported by each
// configured translation provider.
type Catalog struct {
	providers map[Provider]providerLanguages
}

// NewCatalog validates and copies provider capabilities. The returned catalog
// owns its data and is safe for concurrent reads without locking.
func NewCatalog(capabilities map[Provider]Capabilities) (*Catalog, error) {
	if len(capabilities) == 0 {
		return nil, errors.New("language catalog must contain at least one provider")
	}

	catalog := &Catalog{providers: make(map[Provider]providerLanguages, len(capabilities))}
	for provider, configured := range capabilities {
		if strings.TrimSpace(string(provider)) == "" {
			return nil, errors.New("language provider must not be empty")
		}

		source, sourceValues, err := makeRegistry(configured.Source)
		if err != nil {
			return nil, fmt.Errorf("%s source languages: %w", provider, err)
		}
		target, targetValues, err := makeRegistry(configured.Target)
		if err != nil {
			return nil, fmt.Errorf("%s target languages: %w", provider, err)
		}

		catalog.providers[provider] = providerLanguages{
			source: source,
			target: target,
			capabilities: Capabilities{
				Source:     sourceValues,
				Target:     targetValues,
				AutoDetect: configured.AutoDetect,
			},
		}
	}

	return catalog, nil
}

func makeRegistry(values []string) (map[string]string, []string, error) {
	if len(values) == 0 {
		return nil, nil, errors.New("must contain at least one language")
	}

	registry := make(map[string]string, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, nil, errors.New("must not contain an empty language")
		}

		key := strings.ToLower(value)
		if _, exists := registry[key]; exists {
			return nil, nil, fmt.Errorf("contains duplicate language %q", value)
		}
		registry[key] = value
		normalized = append(normalized, value)
	}

	sort.Slice(normalized, func(i, j int) bool {
		return strings.ToLower(normalized[i]) < strings.ToLower(normalized[j])
	})
	return registry, normalized, nil
}

// ResolveSource returns the provider's configured spelling for a source
// language supplied by a client.
func (c *Catalog) ResolveSource(provider Provider, language string) (string, bool) {
	configured, ok := c.providers[provider]
	if !ok {
		return "", false
	}
	resolved, ok := configured.source[strings.ToLower(strings.TrimSpace(language))]
	return resolved, ok
}

// ResolveTarget returns the provider's configured spelling for a target
// language supplied by a client.
func (c *Catalog) ResolveTarget(provider Provider, language string) (string, bool) {
	configured, ok := c.providers[provider]
	if !ok {
		return "", false
	}
	resolved, ok := configured.target[strings.ToLower(strings.TrimSpace(language))]
	return resolved, ok
}

// SupportsAutoDetect reports whether source_lang may be omitted for a provider.
func (c *Catalog) SupportsAutoDetect(provider Provider) bool {
	configured, ok := c.providers[provider]
	return ok && configured.capabilities.AutoDetect
}

// Snapshot returns a detached copy suitable for JSON serialization.
func (c *Catalog) Snapshot() map[Provider]Capabilities {
	result := make(map[Provider]Capabilities, len(c.providers))
	for provider, configured := range c.providers {
		result[provider] = Capabilities{
			Source:     append([]string(nil), configured.capabilities.Source...),
			Target:     append([]string(nil), configured.capabilities.Target...),
			AutoDetect: configured.capabilities.AutoDetect,
		}
	}
	return result
}
