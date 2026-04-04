package sdk

import (
	"encoding/json"
	"strconv"
)

func (c *contextImpl) CaseGetFloat(path string) float64 {
	raw, err := c.CaseGet(path)
	if err != nil {
		return 0
	}
	var n float64
	if err := json.Unmarshal([]byte(raw), &n); err == nil {
		return n
	}
	n, _ = strconv.ParseFloat(raw, 64)
	return n
}

func (c *contextImpl) CaseGetString(path string) string {
	raw, err := c.CaseGet(path)
	if err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		return s
	}
	return raw
}

func (c *contextImpl) CaseGetBool(path string) bool {
	raw, err := c.CaseGet(path)
	if err != nil {
		return false
	}
	var b bool
	if err := json.Unmarshal([]byte(raw), &b); err == nil {
		return b
	}
	b, _ = strconv.ParseBool(raw)
	return b
}

func (c *contextImpl) ConfigFloat(key string) float64 {
	raw := c.Config(key)
	if raw == "" {
		return 0
	}
	n, _ := strconv.ParseFloat(raw, 64)
	return n
}

func (c *contextImpl) ConfigInt(key string) int {
	raw := c.Config(key)
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
}

func (c *contextImpl) ConfigBool(key string) bool {
	raw := c.Config(key)
	if raw == "" {
		return false
	}
	b, _ := strconv.ParseBool(raw)
	return b
}
