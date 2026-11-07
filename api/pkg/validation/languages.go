package validation

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

var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrSourceRequired      = errors.New("source language is required")
	ErrUnsupportedSource   = errors.New("unsupported source language")
	ErrTargetRequired      = errors.New("target language is required")
	ErrUnsupportedTarget   = errors.New("unsupported target language")
)

// Capabilities describes the source and target languages accepted by one
// provider. Values retain provider spelling while matching input without case.
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

// LanguageSelection is a provider and its normalized language values.
type LanguageSelection struct {
	Provider Provider
	Source   string
	Target   string
}

// Catalog is an immutable registry of provider language capabilities.
type Catalog struct {
	providers map[Provider]providerLanguages
}

// NewCatalog validates and copies provider capabilities. The returned catalog
// owns its data and is safe for concurrent reads.
func NewCatalog(capabilities map[Provider]Capabilities) (*Catalog, error) {
	if len(capabilities) == 0 {
		return nil, errors.New("language catalog must contain at least one provider")
	}

	catalog := &Catalog{providers: make(map[Provider]providerLanguages, len(capabilities))}
	for provider, configured := range capabilities {
		providerName := strings.ToLower(strings.TrimSpace(string(provider)))
		if providerName == "" {
			return nil, errors.New("language provider must not be empty")
		}
		provider = Provider(providerName)
		if _, exists := catalog.providers[provider]; exists {
			return nil, fmt.Errorf("duplicate language provider %q", provider)
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

// ResolveProvider normalizes a provider name and verifies that it is present
// in the catalog.
func (c *Catalog) ResolveProvider(provider string) (Provider, bool) {
	if c == nil {
		return "", false
	}
	normalized := Provider(strings.ToLower(strings.TrimSpace(provider)))
	_, ok := c.providers[normalized]
	return normalized, ok
}

// ResolveSource returns provider spelling for a supported source language.
func (c *Catalog) ResolveSource(provider Provider, language string) (string, bool) {
	if c == nil || strings.TrimSpace(string(provider)) == "" {
		return "", false
	}
	configured, ok := c.providers[Provider(strings.ToLower(strings.TrimSpace(string(provider))))]
	if !ok {
		return "", false
	}
	resolved, ok := configured.source[strings.ToLower(strings.TrimSpace(language))]
	return resolved, ok
}

// ResolveTarget returns provider spelling for a supported target language.
func (c *Catalog) ResolveTarget(provider Provider, language string) (string, bool) {
	if c == nil || strings.TrimSpace(string(provider)) == "" {
		return "", false
	}
	configured, ok := c.providers[Provider(strings.ToLower(strings.TrimSpace(string(provider))))]
	if !ok {
		return "", false
	}
	resolved, ok := configured.target[strings.ToLower(strings.TrimSpace(language))]
	return resolved, ok
}

// SupportsAutoDetect reports whether source may be omitted for provider.
func (c *Catalog) SupportsAutoDetect(provider Provider) bool {
	if c == nil || strings.TrimSpace(string(provider)) == "" {
		return false
	}
	configured, ok := c.providers[Provider(strings.ToLower(strings.TrimSpace(string(provider))))]
	return ok && configured.capabilities.AutoDetect
}

// ValidateSelection validates and normalizes a provider-language combination.
func (c *Catalog) ValidateSelection(providerName, source, target string) (LanguageSelection, error) {
	if c == nil {
		return LanguageSelection{}, errors.New("language catalog is nil")
	}

	provider, ok := c.ResolveProvider(providerName)
	if !ok {
		return LanguageSelection{}, ErrUnsupportedProvider
	}

	normalizedSource := ""
	if strings.TrimSpace(source) == "" {
		if !c.SupportsAutoDetect(provider) {
			return LanguageSelection{}, ErrSourceRequired
		}
	} else {
		var supported bool
		normalizedSource, supported = c.ResolveSource(provider, source)
		if !supported {
			return LanguageSelection{}, ErrUnsupportedSource
		}
	}

	if strings.TrimSpace(target) == "" {
		return LanguageSelection{}, ErrTargetRequired
	}
	normalizedTarget, ok := c.ResolveTarget(provider, target)
	if !ok {
		return LanguageSelection{}, ErrUnsupportedTarget
	}

	return LanguageSelection{
		Provider: provider,
		Source:   normalizedSource,
		Target:   normalizedTarget,
	}, nil
}

// ValidateTarget validates and normalizes a target for a fixed provider.
func (c *Catalog) ValidateTarget(provider Provider, target string) (string, error) {
	if c == nil {
		return "", errors.New("language catalog is nil")
	}
	if _, ok := c.ResolveProvider(string(provider)); !ok {
		return "", ErrUnsupportedProvider
	}
	if strings.TrimSpace(target) == "" {
		return "", ErrTargetRequired
	}
	normalized, ok := c.ResolveTarget(provider, target)
	if !ok {
		return "", ErrUnsupportedTarget
	}
	return normalized, nil
}

// ValidateSource validates and normalizes a source for a fixed provider. An
// empty source is accepted only when that provider supports auto-detection.
func (c *Catalog) ValidateSource(provider Provider, source string) (string, error) {
	if c == nil {
		return "", errors.New("language catalog is nil")
	}
	if _, ok := c.ResolveProvider(string(provider)); !ok {
		return "", ErrUnsupportedProvider
	}
	if strings.TrimSpace(source) == "" {
		if c.SupportsAutoDetect(provider) {
			return "", nil
		}
		return "", ErrSourceRequired
	}
	normalized, ok := c.ResolveSource(provider, source)
	if !ok {
		return "", ErrUnsupportedSource
	}
	return normalized, nil
}

// Snapshot returns a detached copy suitable for JSON serialization.
func (c *Catalog) Snapshot() map[Provider]Capabilities {
	if c == nil {
		return map[Provider]Capabilities{}
	}
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
