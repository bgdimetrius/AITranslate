package languages

import "github.com/bgdimetrius/AITranslate/pkg/validation"

// Provider and Capabilities remain available from the HTTP feature package for
// compatibility. Their implementation and validation live in pkg/validation.
type Provider = validation.Provider
type Capabilities = validation.Capabilities

const (
	ProviderChatGPT = validation.ProviderChatGPT
	ProviderDeepL   = validation.ProviderDeepL
)

// Catalog is the languages HTTP adapter around the shared validation catalog.
type Catalog struct {
	validator *validation.Catalog
}

// NewCatalog constructs the single shared provider/language registry.
func NewCatalog(capabilities map[Provider]Capabilities) (*Catalog, error) {
	validator, err := validation.NewCatalog(capabilities)
	if err != nil {
		return nil, err
	}
	return &Catalog{validator: validator}, nil
}

// Validator exposes the pure catalog for injection into other HTTP features.
func (c *Catalog) Validator() *validation.Catalog {
	if c == nil {
		return nil
	}
	return c.validator
}

// ResolveSource matches source-language input without case and returns the
// spelling configured for provider.
func (c *Catalog) ResolveSource(provider Provider, language string) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.validator.ResolveSource(provider, language)
}

// ResolveTarget matches target-language input without case and returns the
// spelling configured for provider.
func (c *Catalog) ResolveTarget(provider Provider, language string) (string, bool) {
	if c == nil {
		return "", false
	}
	return c.validator.ResolveTarget(provider, language)
}

// SupportsAutoDetect reports whether provider accepts an omitted source
// language.
func (c *Catalog) SupportsAutoDetect(provider Provider) bool {
	return c != nil && c.validator.SupportsAutoDetect(provider)
}

// Snapshot returns a detached representation of the configured capabilities.
func (c *Catalog) Snapshot() map[Provider]Capabilities {
	if c == nil {
		return map[Provider]Capabilities{}
	}
	return c.validator.Snapshot()
}
