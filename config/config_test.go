package config

import (
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		Server: ServerConfig{
			Port:       8080,
			MaxStreams: 256,
		},
		CacheMaxEntries: 10000,
		RateLimits: RateLimitsConfig{
			Default:   300,
			Expensive: 20,
			Injection: 10,
			Script:    5,
			Streaming: 5,
			Debug:     1,
		},
		Chains: ChainsConfig{
			Tezos: ChainConfig{
				Networks: map[string]NetworkConfig{
					"mainnet": {
						Nodes: []NodeConfig{
							{Name: "node1", URL: "http://10.0.0.1:8732"},
						},
					},
				},
			},
		},
	}
}

func TestValidateValid(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidatePortRange(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Port = 0
	assertValidationError(t, &cfg, "server.port")

	cfg.Server.Port = 70000
	assertValidationError(t, &cfg, "server.port")
}

func TestValidateMaxStreams(t *testing.T) {
	cfg := validConfig()
	cfg.Server.MaxStreams = 0
	assertValidationError(t, &cfg, "max_streams")
}

func TestValidateCacheMaxEntries(t *testing.T) {
	cfg := validConfig()
	cfg.CacheMaxEntries = 0
	assertValidationError(t, &cfg, "cache_max_entries")
}

func TestValidateRateLimits(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimits.Default = 0
	assertValidationError(t, &cfg, "rate_limits.default")

	cfg = validConfig()
	cfg.RateLimits.Debug = -1
	assertValidationError(t, &cfg, "rate_limits.debug")
}

func TestValidateRateLimitsDisabledSkipsCheck(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimits.Disabled = true
	cfg.RateLimits.Default = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled rate limits should skip validation, got: %v", err)
	}
}

func TestValidateNoNetworks(t *testing.T) {
	cfg := validConfig()
	cfg.Chains = ChainsConfig{}
	assertValidationError(t, &cfg, "at least one network")
}

func TestValidateEmptyNodes(t *testing.T) {
	cfg := validConfig()
	cfg.Chains.Tezos.Networks["mainnet"] = NetworkConfig{Nodes: nil}
	assertValidationError(t, &cfg, "at least one node")
}

func TestValidateNodeMissingName(t *testing.T) {
	cfg := validConfig()
	cfg.Chains.Tezos.Networks["mainnet"] = NetworkConfig{
		Nodes: []NodeConfig{{Name: "", URL: "http://localhost:8732"}},
	}
	assertValidationError(t, &cfg, "name is required")
}

func TestValidateNodeMissingURL(t *testing.T) {
	cfg := validConfig()
	cfg.Chains.Tezos.Networks["mainnet"] = NetworkConfig{
		Nodes: []NodeConfig{{Name: "n1", URL: ""}},
	}
	assertValidationError(t, &cfg, "url is required")
}

func TestValidateNodeInvalidURL(t *testing.T) {
	cfg := validConfig()
	cfg.Chains.Tezos.Networks["mainnet"] = NetworkConfig{
		Nodes: []NodeConfig{{Name: "n1", URL: "not-a-url"}},
	}
	assertValidationError(t, &cfg, "invalid url")
}

func TestValidateNodeDuplicateName(t *testing.T) {
	cfg := validConfig()
	cfg.Chains.Tezos.Networks["mainnet"] = NetworkConfig{
		Nodes: []NodeConfig{
			{Name: "dup", URL: "http://a:1"},
			{Name: "dup", URL: "http://b:2"},
		},
	}
	assertValidationError(t, &cfg, "duplicate node name")
}

func TestValidateFallbackInvalidURL(t *testing.T) {
	cfg := validConfig()
	net := cfg.Chains.Tezos.Networks["mainnet"]
	net.Fallbacks = []string{"ftp://bad"}
	cfg.Chains.Tezos.Networks["mainnet"] = net
	assertValidationError(t, &cfg, "invalid url")
}

func assertValidationError(t *testing.T, cfg *Config, substr string) {
	t.Helper()
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}
