/*
Copyright 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package upgrade

import (
	"fmt"
	"strings"

	semver "github.com/blang/semver/v4"
)

// ParseVersion validates and normalizes a DirectoryService version.
func ParseVersion(value string) (semver.Version, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if value == "" {
		return semver.Version{}, fmt.Errorf("version is empty")
	}
	version, err := semver.Parse(value)
	if err != nil {
		return semver.Version{}, fmt.Errorf("invalid semantic version %q: %w", value, err)
	}
	return version, nil
}

// ValidateTarget rejects downgrades and major-version changes.
func ValidateTarget(current, target string) error {
	currentVersion, err := ParseVersion(current)
	if err != nil {
		return fmt.Errorf("current version: %w", err)
	}
	targetVersion, err := ParseVersion(target)
	if err != nil {
		return fmt.Errorf("target version: %w", err)
	}
	if targetVersion.LT(currentVersion) {
		return fmt.Errorf("version downgrade from %s to %s is not supported", current, target)
	}
	if targetVersion.Major != currentVersion.Major {
		return fmt.Errorf("major version upgrade from %s to %s is not supported", current, target)
	}
	return nil
}

// ExtractVersionFromImage extracts a semantic version from an image tag.
func ExtractVersionFromImage(image string) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", fmt.Errorf("image is empty")
	}
	lastSlash := strings.LastIndex(image, "/")
	name := image[lastSlash+1:]
	colon := strings.LastIndex(name, ":")
	if colon < 0 || colon == len(name)-1 {
		return "", fmt.Errorf("image %q has no semantic version tag", image)
	}
	tag := name[colon+1:]
	if strings.Contains(tag, "@") {
		return "", fmt.Errorf("image %q uses a digest instead of a version tag", image)
	}
	if _, err := ParseVersion(tag); err != nil {
		return "", fmt.Errorf("image %q: %w", image, err)
	}
	return strings.TrimPrefix(tag, "v"), nil
}
