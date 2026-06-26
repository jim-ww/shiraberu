package providers

import (
	"maps"
	"slices"
)

var nameProvider = map[string]SearchProvider{}

func register(provider SearchProvider) {
	nameProvider[provider.Name()] = provider
}

func All() []string {
	return slices.Collect(maps.Keys(nameProvider))
}

func GetProviderFromString(name string) (provider SearchProvider, valid bool) {
	provider, exists := nameProvider[name]
	return provider, exists
}
