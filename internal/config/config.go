package config

import (
	"bufio"
	"os"
	"strings"
)

// Load reads KEY=VALUE lines from a .env file into the process environment.
// Existing environment variables win, so real env vars override the file.
func Load(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env is fine
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

// env returns the value for key, or def if unset/empty.
func Get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Port is the listen port for the local dev server.
func Port() string { return Get("PORT", "8080") }

// DevMode reports whether this is a local, non-production deployment. It fails
// safe: only an explicitly development-ish ENV counts, so a deployment that
// forgets to set ENV is treated as production and won't echo reset tokens in
// API responses.
func DevMode() bool {
	switch strings.ToLower(Get("ENV", "production")) {
	case "development", "dev", "local", "test":
		return true
	}
	return false
}
