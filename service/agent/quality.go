package agent

import (
	"log/slog"
	"regexp"
	"strings"
)

// HardFieldCorrector 硬字段自动修正器。
// 比对 LLM 回答中的价格、金额、资料名与 facts 中的对应值，不一致则替换。
type HardFieldCorrector struct{}

// Correct 修正 LLM 回答中的硬字段偏差。
// 所有 LLM 回答均为非流式获取，修正后再发往前端。
func (c *HardFieldCorrector) Correct(answer string, facts []FactItem, requestID string) string {
	correctValues := extractHardFields(facts)
	claimedValues := extractHardFieldsFromText(answer)

	result := answer
	for _, claimed := range claimedValues {
		if correct, ok := correctValues[claimed.field]; ok && correct != claimed.value {
			result = strings.Replace(result, claimed.raw, correct, 1)
			slog.Warn("agent quality 硬字段修正",
				"request_id", requestID,
				"field", claimed.field,
				"claimed", claimed.value,
				"corrected", correct,
			)
		}
	}
	return result
}

type hardField struct {
	field string
	value string
	raw   string
}

// extractHardFields 从 facts 中提取正确的硬字段值（价格、资料名）。
func extractHardFields(facts []FactItem) map[string]string {
	result := make(map[string]string)
	for _, f := range facts {
		// 提取价格：¥XX.XX 或 XX.XX元
		if re := regexp.MustCompile(`¥[\d.]+`); re.MatchString(f.Content) {
			result["price"] = re.FindString(f.Content)
		}
		// 提取资料名：书名号内容
		if re := regexp.MustCompile(`《([^》]+)》`); re.MatchString(f.Content) {
			result["title"] = re.FindString(f.Content)
		}
	}
	return result
}

// extractHardFieldsFromText 从 LLM 回答中提取声称的硬字段（价格、资料名）。
func extractHardFieldsFromText(text string) []hardField {
	var fields []hardField
	if re := regexp.MustCompile(`¥[\d.]+`); re.MatchString(text) {
		raw := re.FindString(text)
		fields = append(fields, hardField{field: "price", value: raw, raw: raw})
	}
	if re := regexp.MustCompile(`《([^》]+)》`); re.MatchString(text) {
		raw := re.FindString(text)
		fields = append(fields, hardField{field: "title", value: raw, raw: raw})
	}
	return fields
}
