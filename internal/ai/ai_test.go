package ai

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_CustomSystemNoRag(t *testing.T) {
	p := &Prompt{
		System:   "Custom system prompt.",
		Question: "What is Bitcoin?",
	}
	got, err := BuildSystemPrompt(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Custom system prompt." {
		t.Errorf("expected custom system prompt unchanged, got %q", got)
	}
}

func TestBuildSystemPrompt_CustomSystemWithRag(t *testing.T) {
	p := &Prompt{
		System:   "Answer using ONLY the tool results below.",
		Rag:      []string{"### news\nA current headline with source context."},
		Question: "What is the latest headline?",
	}
	got, err := BuildSystemPrompt(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "Answer using ONLY the tool results below.") {
		t.Errorf("expected custom system prompt in output, got %q", got)
	}
	if !strings.Contains(got, "A current headline with source context.") {
		t.Errorf("expected RAG content in output, got %q", got)
	}
	if !strings.Contains(got, "Current context") {
		t.Errorf("expected context header in output, got %q", got)
	}
	if strings.Contains(got, "live market data") {
		t.Fatalf("prompt still claims removed live market data: %s", got)
	}
}

func TestBuildSystemPrompt_DefaultTemplateWithRag(t *testing.T) {
	p := &Prompt{
		Rag:      []string{"### news\nA current headline with source context."},
		Question: "What is the latest headline?",
	}
	got, err := BuildSystemPrompt(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "A current headline with source context.") {
		t.Errorf("expected RAG content in default template output, got %q", got)
	}
	if strings.Contains(got, "live market data") {
		t.Fatalf("prompt still claims removed live market data: %s", got)
	}
	if strings.Contains(got, "data provided in context is current and live") {
		t.Fatalf("prompt still claims prices in context are live: %s", got)
	}
}
