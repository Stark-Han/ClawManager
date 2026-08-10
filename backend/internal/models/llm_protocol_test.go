package models

import "testing"

func TestResolveLLMProtocolType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		providerType string
		protocolType string
		want         string
		wantErr      bool
	}{
		{
			name:         "local defaults to openai compatible",
			providerType: ProviderTypeLocal,
			want:         ProtocolTypeOpenAICompatible,
		},
		{
			name:         "local accepts anthropic",
			providerType: ProviderTypeLocal,
			protocolType: ProtocolTypeAnthropic,
			want:         ProtocolTypeAnthropic,
		},
		{
			name:         "local normalizes openai to openai compatible",
			providerType: ProviderTypeLocal,
			protocolType: ProtocolTypeOpenAI,
			want:         ProtocolTypeOpenAICompatible,
		},
		{
			name:         "local rejects unsupported protocol",
			providerType: ProviderTypeLocal,
			protocolType: ProtocolTypeGoogle,
			wantErr:      true,
		},
		{
			name:         "anthropic keeps its own protocol",
			providerType: ProviderTypeAnthropic,
			protocolType: ProtocolTypeOpenAICompatible,
			want:         ProtocolTypeAnthropic,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveLLMProtocolType(tc.providerType, tc.protocolType)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got protocol %q", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
