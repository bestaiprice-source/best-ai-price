package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	DefaultAuthProvidersPaths = []string{
		"/app/data/auth_providers.json",
		"./data/auth_providers.json",
		"/data/auth_providers.json",
		"/app/data/parser/auth_providers.json",
		"/srv/parser-data/data/auth_providers.json",
		"/root/bestai_prices_parser/public/data/auth_providers.json",
	}
	ErrInvalidJSON = errors.New("invalid json content")
)

// AuthProvidersService manages reading and writing auth_providers.json
type AuthProvidersService struct {
	mu          sync.RWMutex
	customPaths []string
}

// NewAuthProvidersService creates a new AuthProvidersService instance
func NewAuthProvidersService() *AuthProvidersService {
	return &AuthProvidersService{}
}

// NewAuthProvidersServiceWithPaths creates a new AuthProvidersService with custom file paths (primarily for testing)
func NewAuthProvidersServiceWithPaths(paths []string) *AuthProvidersService {
	return &AuthProvidersService{
		customPaths: paths,
	}
}

func (s *AuthProvidersService) getPaths() []string {
	if len(s.customPaths) > 0 {
		return s.customPaths
	}
	return DefaultAuthProvidersPaths
}

// Save validates JSON and writes the data to candidate auth_providers.json locations
func (s *AuthProvidersService) Save(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(data) == 0 || !json.Valid(data) {
		return ErrInvalidJSON
	}

	paths := s.getPaths()
	written := 0
	var lastErr error

	for _, p := range paths {
		dir := filepath.Dir(p)
		// Try to create parent directory if possible
		if err := os.MkdirAll(dir, 0755); err != nil {
			lastErr = err
			continue
		}

		if err := os.WriteFile(p, data, 0644); err != nil {
			lastErr = err
			continue
		}
		written++
	}

	if written == 0 {
		// Fallback: try writing to local ./data/auth_providers.json
		fallbackPath := "./data/auth_providers.json"
		if err := os.MkdirAll(filepath.Dir(fallbackPath), 0755); err == nil {
			if err := os.WriteFile(fallbackPath, data, 0644); err == nil {
				return nil
			}
		}
		if lastErr != nil {
			return fmt.Errorf("failed to save auth_providers.json: %w", lastErr)
		}
		return errors.New("failed to write auth_providers.json to any target path")
	}

	return nil
}

// Read reads auth_providers.json returning the newest content among existing candidate locations
func (s *AuthProvidersService) Read() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := s.getPaths()
	var latestData []byte
	var latestModTime time.Time
	found := false

	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			data, err := os.ReadFile(p)
			if err == nil {
				if !found || info.ModTime().After(latestModTime) {
					latestData = data
					latestModTime = info.ModTime()
					found = true
				}
			}
		}
	}

	if found {
		return latestData, nil
	}

	return nil, os.ErrNotExist
}
