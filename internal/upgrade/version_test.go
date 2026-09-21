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

import "testing"

func TestParseVersion(t *testing.T) {
	for _, value := range []string{"3.0.0", "v3.1.2", "3.1.0-rc.1"} {
		if _, err := ParseVersion(value); err != nil {
			t.Errorf("ParseVersion(%q): %v", value, err)
		}
	}
	for _, value := range []string{"", "latest", "3.0", "v3"} {
		if _, err := ParseVersion(value); err == nil {
			t.Errorf("ParseVersion(%q) succeeded", value)
		}
	}
}

func TestValidateTarget(t *testing.T) {
	for _, test := range []struct {
		name    string
		current string
		target  string
		valid   bool
	}{
		{name: "patch", current: "3.0.0", target: "3.0.1", valid: true},
		{name: "minor", current: "3.0.0", target: "3.1.0", valid: true},
		{name: "same", current: "3.0.0", target: "3.0.0", valid: true},
		{name: "downgrade", current: "3.1.0", target: "3.0.0", valid: false},
		{name: "major", current: "3.0.0", target: "4.0.0", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTarget(test.current, test.target)
			if (err == nil) != test.valid {
				t.Fatalf("ValidateTarget(%q, %q) error = %v", test.current, test.target, err)
			}
		})
	}
	if err := ValidateTransition("3.0.0", "4.0.0", "3.0.0->4.0.0"); err != nil {
		t.Fatalf("major upgrade with approval: %v", err)
	}
	if err := ValidateTransition("3.0.0", "4.0.0", ""); err == nil {
		t.Fatal("major upgrade without approval succeeded")
	}
	if err := ValidateTransition("4.0.0", "3.0.0", "4.0.0->3.0.0"); err != nil {
		t.Fatalf("major downgrade with approval: %v", err)
	}
	if err := ValidateTransition("3.2.0", "3.1.0", "4.0.0->3.1.0"); err == nil {
		t.Fatal("minor downgrade with wrong approval succeeded")
	}
	if err := ValidateTransition("3.1.2", "3.1.1", ""); err != nil {
		t.Fatalf("patch downgrade: %v", err)
	}
}

func TestExtractVersionFromImage(t *testing.T) {
	version, err := ExtractVersionFromImage("quay.io/389ds/dirsrv:3.1.0")
	if err != nil || version != "3.1.0" {
		t.Fatalf("got version %q, error %v", version, err)
	}
	for _, image := range []string{"quay.io/389ds/dirsrv:latest", "quay.io/389ds/dirsrv", "quay.io/389ds/dirsrv@sha256:abc"} {
		if _, err := ExtractVersionFromImage(image); err == nil {
			t.Errorf("ExtractVersionFromImage(%q) succeeded", image)
		}
	}
}

func TestRequiresBackup(t *testing.T) {
	for _, test := range []struct {
		current, target string
		required        bool
	}{
		{"3.1.1", "3.1.2", false},
		{"3.1.2", "3.1.1", false},
		{"3.1.0", "3.2.0", false},
		{"3.2.0", "3.1.0", true},
		{"3.1.0", "4.0.0", true},
		{"4.0.0", "3.1.0", true},
	} {
		if got := RequiresBackup(test.current, test.target); got != test.required {
			t.Errorf("RequiresBackup(%q, %q) = %t, want %t", test.current, test.target, got, test.required)
		}
	}
}
