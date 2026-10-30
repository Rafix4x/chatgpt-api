package pow

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

const DefaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

func FNV1a(s string) string {
	var hash uint32 = 0x811C9DC5
	for i := 0; i < len(s); i++ {
		hash ^= uint32(s[i])
		hash *= 0x01000193
	}
	hash ^= hash >> 16
	hash *= 0x85EBCA6B
	hash ^= hash >> 13
	hash *= 0xC2B2AE35
	hash ^= hash >> 16
	return fmt.Sprintf("%08x", hash)
}

type Config []any

func BuildConfig(buildID, deviceID, ua string) Config {
	if ua == "" {
		ua = DefaultUA
	}
	if deviceID == "" {
		deviceID = RandomUUID()
	}
	return Config{
		4880,
		time.Now().Format("Mon Jan 02 2006"),
		4294705152,
		rand.Float64(),
		ua,
		nil,
		buildID,
		"en-US",
		"en-US,en",
		rand.Float64(),
		"webkitGetUserMedia",
		"location",
		"window",
		800 + rand.Float64()*600,
		deviceID,
		"",
		20,
		time.Now().UnixMilli(),
	}
}

func (c Config) Encode() (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return "gAAAAAC" + base64.StdEncoding.EncodeToString(raw), nil
}

func (c Config) Solve(seed, difficulty string, maxAttempts int) (string, error) {
	if difficulty == "" {
		return "", fmt.Errorf("empty difficulty")
	}
	if maxAttempts <= 0 {
		maxAttempts = 500000
	}
	start := time.Now()
	for i := 0; i < maxAttempts; i++ {
		c[3] = i
		c[9] = float64(time.Since(start).Milliseconds())
		raw, err := json.Marshal(c)
		if err != nil {
			return "", err
		}
		encoded := base64.StdEncoding.EncodeToString(raw)
		if FNV1a(seed + encoded)[:len(difficulty)] <= difficulty {
			return "gAAAAAB" + encoded + "~S", nil
		}
	}
	return "", fmt.Errorf("no proof found in %d attempts", maxAttempts)
}

func RandomUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func EchoLogs() string {
	t1 := 6000 + rand.Intn(3000)
	t2 := t1 + rand.Intn(1200)
	return fmt.Sprintf("0,%d,1,%d", t1, t2)
}
