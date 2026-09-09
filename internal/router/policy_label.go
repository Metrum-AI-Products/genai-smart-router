// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import "strings"

const unsafePolicyClassLabel = "unsafe_class_label"

func safePolicyClassLabel(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 64 {
		label := unsafePolicyClassLabel
		return &label
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == ':':
		default:
			label := unsafePolicyClassLabel
			return &label
		}
	}
	label := value
	return &label
}
