package router

import "testing"

func TestResolveSwaggerEnabled(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		want   bool
		reason string
	}{
		{
			name:   "defaults to enabled in non production",
			want:   true,
			reason: "non-production runtime",
		},
		{
			name: "release mode alone still enables swagger",
			env: map[string]string{
				"GIN_MODE": "release",
			},
			want:   true,
			reason: "GIN_MODE=release without production APP_ENV/ENV; defaulting to enabled",
		},
		{
			name: "production app env disables swagger",
			env: map[string]string{
				"APP_ENV": "production",
			},
			want:   false,
			reason: "APP_ENV/ENV indicates production",
		},
		{
			name: "explicit enable overrides production fallback",
			env: map[string]string{
				"APP_ENV":                 "production",
				"WEKNORA_SWAGGER_ENABLED": "true",
			},
			want:   true,
			reason: "WEKNORA_SWAGGER_ENABLED=true",
		},
		{
			name: "explicit disable overrides non production fallback",
			env: map[string]string{
				"WEKNORA_SWAGGER_ENABLED": "false",
			},
			want:   false,
			reason: "WEKNORA_SWAGGER_ENABLED=false",
		},
		{
			name: "legacy alias still works",
			env: map[string]string{
				"SWAGGER_ENABLED": "true",
			},
			want:   true,
			reason: "SWAGGER_ENABLED=true",
		},
		{
			name: "invalid override falls back to production rule",
			env: map[string]string{
				"APP_ENV":                 "production",
				"WEKNORA_SWAGGER_ENABLED": "maybe",
			},
			want:   false,
			reason: "invalid WEKNORA_SWAGGER_ENABLED=\"maybe\"; falling back to disabled because APP_ENV/ENV indicates production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"APP_ENV", "ENV", "GIN_MODE", "WEKNORA_SWAGGER_ENABLED", "SWAGGER_ENABLED"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			got, reason := resolveSwaggerEnabled()
			if got != tt.want {
				t.Fatalf("resolveSwaggerEnabled() = %v, want %v (reason=%q)", got, tt.want, reason)
			}
			if reason != tt.reason {
				t.Fatalf("reason = %q, want %q", reason, tt.reason)
			}
		})
	}
}
