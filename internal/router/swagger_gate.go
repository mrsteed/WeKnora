package router

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	envSwaggerEnabled       = "WEKNORA_SWAGGER_ENABLED"
	legacyEnvSwaggerEnabled = "SWAGGER_ENABLED"
)

// resolveSwaggerEnabled decides whether Swagger routes should be mounted.
//
// Gin's release mode only controls framework behavior (logging/debug checks);
// it is not a reliable production/non-production signal. Local dev often runs
// with GIN_MODE=release for parity, and coupling Swagger to that flag caused
// `/swagger/index.html` to disappear unexpectedly. We therefore use an explicit
// override first, then fall back to environment intent (APP_ENV / ENV).
func resolveSwaggerEnabled() (bool, string) {
	if enabled, reason, ok := swaggerEnabledOverride(); ok {
		return enabled, reason
	}

	if isProductionRuntimeEnv() {
		return false, "APP_ENV/ENV indicates production"
	}

	if strings.EqualFold(strings.TrimSpace(os.Getenv("GIN_MODE")), "release") {
		return true, "GIN_MODE=release without production APP_ENV/ENV; defaulting to enabled"
	}

	return true, "non-production runtime"
}

func swaggerEnabledOverride() (bool, string, bool) {
	for _, name := range []string{envSwaggerEnabled, legacyEnvSwaggerEnabled} {
		raw, ok := os.LookupEnv(name)
		if !ok {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		enabled, err := strconv.ParseBool(value)
		if err == nil {
			return enabled, fmt.Sprintf("%s=%s", name, strings.ToLower(value)), true
		}
		fallbackEnabled, fallbackReason := swaggerFallbackReason()
		return fallbackEnabled, fmt.Sprintf("invalid %s=%q; %s", name, value, fallbackReason), true
	}

	return false, "", false
}

func swaggerFallbackReason() (bool, string) {
	if isProductionRuntimeEnv() {
		return false, "falling back to disabled because APP_ENV/ENV indicates production"
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GIN_MODE")), "release") {
		return true, "falling back to enabled because GIN_MODE=release alone no longer disables Swagger"
	}
	return true, "falling back to enabled for non-production runtime"
}

func isProductionRuntimeEnv() bool {
	for _, value := range []string{os.Getenv("APP_ENV"), os.Getenv("ENV")} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "prod", "production":
			return true
		}
	}
	return false
}
