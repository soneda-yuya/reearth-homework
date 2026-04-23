// Package llm fulfils domain.LocationExtractor by prompting Anthropic Claude
// for a location string. The prompt is held in the package so changes to
// extraction logic stay reviewable next to the parsing rules.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Completer is the subset of platform/llm.Client the extractor uses. Tests
// inject a stub; production wires the real Claude client.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Extractor turns a MOFA MailItem into an ExtractResult by asking the LLM
// for the most specific location mentioned.
type Extractor struct {
	client Completer
}

// New builds an Extractor backed by client.
func New(client Completer) *Extractor {
	return &Extractor{client: client}
}

// systemPrompt explains the task once. The exact wording is optimised for
// Haiku — short, deterministic, and ends with a strict format instruction.
const systemPrompt = `あなたは外務省 海外安全情報の本文から発生地名を抽出する地理アシスタントです。

出力は次の JSON のみにしてください (前置き / 説明文を入れないでください):
{"location": "<地名 or 空文字>", "confidence": <0.0-1.0>}

地名は地理的に最も具体的なものを選んでください (例: "東京都新宿区"、"パリ 9 区"、"ジャカルタ南部")。
複数の地名が含まれる場合は事象が発生した代表 1 件を選んでください。
本文に地名が無い、または特定が困難な場合は location を空文字、confidence を 0 にしてください。`

// Extract issues one Complete request per item. A JSON parse failure is
// downgraded to ExtractResult{Location: "", Confidence: 0}, err = nil so
// the caller can still flow into the centroid fallback. Transport / auth
// errors propagate so the use case marks the item as failed.
func (e *Extractor) Extract(ctx context.Context, item domain.MailItem) (domain.ExtractResult, error) {
	user := buildUserPrompt(item)
	raw, err := e.client.Complete(ctx, systemPrompt, user)
	if err != nil {
		return domain.ExtractResult{}, errs.Wrap("llm.extract", errs.KindOf(err), err)
	}

	parsed, ok := parseExtractJSON(raw)
	if !ok {
		// Don't fail the item on a malformed response — let the chain
		// fall back to country centroid.
		return domain.ExtractResult{Location: "", Confidence: 0}, nil
	}
	return parsed, nil
}

// buildUserPrompt feeds the LLM the item fields it actually needs. We don't
// include the LeaveDate or codes — they don't help with geographic disambiguation.
func buildUserPrompt(item domain.MailItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Country: %s (%s)\n", item.CountryName, item.CountryCd)
	if item.AreaName != "" {
		fmt.Fprintf(&b, "Area: %s\n", item.AreaName)
	}
	fmt.Fprintf(&b, "Title: %s\n", item.Title)
	if item.Lead != "" {
		fmt.Fprintf(&b, "Lead: %s\n", item.Lead)
	}
	if item.MainText != "" {
		fmt.Fprintf(&b, "MainText: %s\n", item.MainText)
	}
	return b.String()
}

// parseExtractJSON tolerates surrounding whitespace and code-fence wrappers
// (```json ... ```) that some models emit despite the explicit instruction.
func parseExtractJSON(raw string) (domain.ExtractResult, bool) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var payload struct {
		Location   string  `json:"location"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return domain.ExtractResult{}, false
	}
	return domain.ExtractResult{Location: payload.Location, Confidence: payload.Confidence}, true
}
