package api

import "testing"

func TestChatCompletionsURL(t *testing.T) {
	tests := map[string]string{"http://ai.local": "http://ai.local/v1/chat/completions", "http://ai.local/v1": "http://ai.local/v1/chat/completions", "http://ai.local/openai/v1": "http://ai.local/openai/v1/chat/completions"}
	for in, want := range tests {
		got, err := chatCompletionsURL(in)
		if err != nil || got != want {
			t.Fatalf("%s => %s, %v; want %s", in, got, err, want)
		}
	}
}
