package models

import (
	"fmt"
	"strings"
)

const (
	ProviderTypeOpenAI           = "openai"
	ProviderTypeOpenAICompatible = "openai-compatible"
	ProviderTypeAnthropic        = "anthropic"
	ProviderTypeGoogle           = "google"
	ProviderTypeAzureOpenAI      = "azure-openai"
	ProviderTypeLocal            = "local"

	ProtocolTypeOpenAI           = "openai"
	ProtocolTypeOpenAICompatible = "openai-compatible"
	ProtocolTypeAnthropic        = "anthropic"
	ProtocolTypeGoogle           = "google"
	ProtocolTypeAzureOpenAI      = "azure-openai"
)

func normalizeLLMType(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

// ResolveLLMProtocolType validates and normalizes the effective transport protocol for a model.
func ResolveLLMProtocolType(providerType, protocolType string) (string, error) {
	normalizedProviderType := normalizeLLMType(providerType)
	normalizedProtocolType := normalizeLLMType(protocolType)

	switch normalizedProviderType {
	case "":
		return "", fmt.Errorf("provider type is required")
	case ProviderTypeLocal:
		switch normalizedProtocolType {
		case "", ProtocolTypeOpenAI, ProtocolTypeOpenAICompatible:
			return ProtocolTypeOpenAICompatible, nil
		case ProtocolTypeAnthropic:
			return ProtocolTypeAnthropic, nil
		default:
			return "", fmt.Errorf("protocol type %s is not supported for local provider", normalizedProtocolType)
		}
	case ProviderTypeOpenAI:
		return ProtocolTypeOpenAI, nil
	case ProviderTypeOpenAICompatible:
		return ProtocolTypeOpenAICompatible, nil
	case ProviderTypeAnthropic:
		return ProtocolTypeAnthropic, nil
	case ProviderTypeGoogle:
		return ProtocolTypeGoogle, nil
	case ProviderTypeAzureOpenAI:
		return ProtocolTypeAzureOpenAI, nil
	default:
		if normalizedProtocolType != "" {
			return normalizedProtocolType, nil
		}
		return normalizedProviderType, nil
	}
}

// ResolveLLMProtocolTypeOrDefault returns a normalized protocol type and falls back to a sensible default.
func ResolveLLMProtocolTypeOrDefault(providerType, protocolType string) string {
	resolved, err := ResolveLLMProtocolType(providerType, protocolType)
	if err == nil {
		return resolved
	}

	switch normalizeLLMType(providerType) {
	case ProviderTypeLocal:
		return ProtocolTypeOpenAICompatible
	case ProviderTypeOpenAI:
		return ProtocolTypeOpenAI
	case ProviderTypeAnthropic:
		return ProtocolTypeAnthropic
	case ProviderTypeGoogle:
		return ProtocolTypeGoogle
	case ProviderTypeAzureOpenAI:
		return ProtocolTypeAzureOpenAI
	case ProviderTypeOpenAICompatible:
		return ProtocolTypeOpenAICompatible
	default:
		return normalizeLLMType(providerType)
	}
}
