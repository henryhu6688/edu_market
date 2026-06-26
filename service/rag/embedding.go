package rag

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"edu_market/config"
	"edu_market/database"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// embedRate Embedding API 令牌桶限流（8次/秒，突发2次）
var embedRate = rate.NewLimiter(8, 2)

// embedTexts 批量 Embedding，并发 3 请求，令牌桶限流。
func embedTexts(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	var mu sync.Mutex
	g := new(errgroup.Group)
	g.SetLimit(3)

	for i, t := range texts {
		i, t := i, t
		g.Go(func() error {
			embedRate.Wait(context.Background())
			vec, err := embedCached(t)
			if err != nil {
				return err
			}
			mu.Lock()
			results[i] = vec
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// embedCached 单文本 Embedding，带 Redis 缓存。
func embedCached(text string) ([]float32, error) {
	key := "emb:" + fmt.Sprintf("%x", md5.Sum([]byte(text)))

	// 1. 查缓存
	if database.RDB != nil {
		if b, err := database.RDB.Get(context.Background(), key).Bytes(); err == nil && len(b) > 0 {
			return bytesToFloats(b), nil
		}
	}

	// 2. 调 API
	vec, err := callEmbeddingAPI(text)
	if err != nil {
		return nil, err
	}

	// 3. 写缓存（TTL 24h）
	if database.RDB != nil {
		database.RDB.SetEx(context.Background(), key, floatsToBytes(vec), 24*time.Hour)
	}
	return vec, nil
}

// callEmbeddingAPI 调 SiliconFlow Embedding API，3 次指数退避重试。
func callEmbeddingAPI(text string) ([]float32, error) {
	apiURL := config.App.Agent.EmbeddingAPIURL
	if apiURL == "" {
		apiURL = "https://api.siliconflow.cn/v1/embeddings"
	}
	model := config.App.Agent.EmbeddingModel
	if model == "" {
		model = "BAAI/bge-m3"
	}

	reqBody := map[string]interface{}{
		"model":           model,
		"input":           text,
		"encoding_format": "float",
	}
	jsonBytes, _ := json.Marshal(reqBody)

	apiKey := config.App.Agent.EmbeddingAPIKey
	if apiKey == "" {
		apiKey = config.App.AI.APIKey
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}

		req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
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
		return result.Data[0].Embedding, nil
	}
	return nil, fmt.Errorf("embedding 重试 3 次后仍失败: %w", lastErr)
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
