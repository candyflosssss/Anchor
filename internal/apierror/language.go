package apierror

import (
	"strconv"
	"strings"
)

type Language string

const (
	English Language = "en"
	Chinese Language = "zh-CN"
)

// Negotiate selects a supported language by quality and then header order.
// English is the compatibility fallback for missing/unsupported preferences.
// Parse at most 32 ranges and 1024 bytes; request language never changes global state.
func Negotiate(header string) Language {
	if len(header) > 1024 {
		return English
	}
	selected, best := English, 0.0
	for index, item := range strings.Split(header, ",") {
		if index >= 32 {
			break
		}
		parts := strings.Split(item, ";")
		tag := strings.ToLower(strings.TrimSpace(parts[0]))
		if tag == "" {
			continue
		}
		quality, valid := 1.0, true
		if len(parts) > 2 {
			continue
		}
		if len(parts) == 2 {
			key, value, ok := strings.Cut(strings.TrimSpace(parts[1]), "=")
			value = strings.TrimSpace(value)
			if !ok || strings.TrimSpace(key) != "q" || !validQuality(value) {
				valid = false
			} else {
				quality, _ = strconv.ParseFloat(value, 64)
			}
		}
		if !valid || quality <= best {
			continue
		}
		lang := English
		switch {
		case tag == "zh" || strings.HasPrefix(tag, "zh-"):
			lang = Chinese
		case tag == "en" || strings.HasPrefix(tag, "en-"), tag == "*":
		default:
			continue
		}
		selected, best = lang, quality
	}
	return selected
}

func validQuality(value string) bool {
	if len(value) == 0 || len(value) > 5 || (value[0] != '0' && value[0] != '1') {
		return false
	}
	if len(value) == 1 {
		return true
	}
	if value[1] != '.' {
		return false
	}
	for _, digit := range value[2:] {
		if digit < '0' || digit > '9' || (value[0] == '1' && digit != '0') {
			return false
		}
	}
	return true
}
