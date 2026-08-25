package realmguard

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Digest fingerprints the numbers a battle depends on.
//
// The browser computes the same value over its own projection and sends it with
// the ledger. If the two disagree the result is refused instead of scored from
// rules the player never actually played against.
func Digest(config Config) (string, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return "", err
	}
	var builder strings.Builder
	canonical(&builder, generic)
	return fnv1a(builder.String()), nil
}

// canonicalNumber renders a float the same way the browser does. Neither
// encoder's shortest representation is trusted, because they disagree on some
// doubles.
func canonicalNumber(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0"
	}
	if value == 0 {
		return "0"
	}
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	text := strconv.FormatFloat(value, 'f', 6, 64)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(text, "0")
		text = strings.TrimSuffix(text, ".")
	}
	if text == "-0" {
		return "0"
	}
	return text
}

func canonical(builder *strings.Builder, value any) {
	switch typed := value.(type) {
	case nil:
		builder.WriteString("null")
	case bool:
		if typed {
			builder.WriteString("true")
		} else {
			builder.WriteString("false")
		}
	case float64:
		builder.WriteString(canonicalNumber(typed))
	case string:
		encoded, _ := json.Marshal(typed)
		builder.Write(encoded)
	case []any:
		builder.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				builder.WriteByte(',')
			}
			canonical(builder, item)
		}
		builder.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		builder.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				builder.WriteByte(',')
			}
			encoded, _ := json.Marshal(key)
			builder.Write(encoded)
			builder.WriteByte(':')
			canonical(builder, typed[key])
		}
		builder.WriteByte('}')
	default:
		builder.WriteString("null")
	}
}

func fnv1a(text string) string {
	const offset = uint64(0xcbf29ce484222325)
	const prime = uint64(0x100000001b3)
	hash := offset
	for index := 0; index < len(text); index++ {
		hash ^= uint64(text[index])
		hash *= prime
	}
	return fmt.Sprintf("%016x", hash)
}
