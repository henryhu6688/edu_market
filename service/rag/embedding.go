package rag

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"edu_market/config"

	"golang.org/x/time/rate"
)

// embedRate Embedding API 令牌桶限流（8次/秒，突发2次）
var embedRate = rate.NewLimiter(8, 2)

// embedTexts 批量调 Embedding API，返回多个文本的向量
func embedTexts(texts []string) ([][]float32, error) {
	apiURL := config.App.Agent.EmbeddingAPIURL
	if apiURL == "" {
		apiURL = "https://api.siliconflow.cn/v1/embeddings"
	}
	model := config.App.Agent.EmbeddingModel
	if model == "" {
		model = "BAAI/bge-large-zh-v1.5"
	}

	input := texts[0]
	if len(texts) > 1 {
		input = joinStrings(texts, "\n")
	}
	reqBody := map[string]interface{}{
		"model":           model,
		"input":           input,
		"encoding_format": "float",
	}
	jsonBytes, _ := json.Marshal(reqBody)

	// 3 次指数退避重试
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}

		embedRate.Wait(context.Background())
		req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		apiKey := config.App.Agent.EmbeddingAPIKey
		if apiKey == "" {
			apiKey = config.App.AI.APIKey
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("embedding API 返回状态 %d: %s", resp.StatusCode, string(body))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("embedding API 错误: status=%d body=%s", resp.StatusCode, string(body))
		}

		var result struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		if len(result.Data) == 0 {
			return nil, fmt.Errorf("embedding 返回空")
		}

		embeddings := make([][]float32, len(result.Data))
		for i, d := range result.Data {
			embeddings[i] = d.Embedding
		}
		return embeddings, nil
	}
	return nil, fmt.Errorf("embedding 重试 3 次后仍失败: %w", lastErr)
}

// joinStrings 字符串连接（等同于 strings.Join）
func joinStrings(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	if len(elems) == 1 {
		return elems[0]
	}
	var b bytes.Buffer
	b.WriteString(elems[0])
	for _, s := range elems[1:] {
		b.WriteString(sep)
		b.WriteString(s)
	}
	return b.String()
}

// cosineSimilarity 余弦相似度（两个等长向量）
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// floatsToBytes []float32 → []byte（小端序，每 float32 4 字节）
func floatsToBytes(v []float32) []byte {
	buf := new(bytes.Buffer)
	for _, f := range v {
		binary.Write(buf, binary.LittleEndian, f)
	}
	return buf.Bytes()
}

// bytesToFloats []byte → []float32（小端序，每 4 字节一个 float32）
func bytesToFloats(b []byte) []float32 {
	buf := bytes.NewReader(b)
	v := make([]float32, len(b)/4)
	for i := range v {
		binary.Read(buf, binary.LittleEndian, &v[i])
	}
	return v
}
