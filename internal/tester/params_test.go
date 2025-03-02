package tester

import (
	"encoding/json"
	"testing"
)

func TestParams(t *testing.T) {
	raw := []byte(`
{
  "name": "test-run-001",
  "duration": "5m",
  "pace": "6000rpm",
  "parallelTesters": 10,
  "timeout": "5s",
  "choice": "roundrobin",
  "reqSchema": "https",
  "reqVersion": "1.1",
  "reqIDHeader": "X-Request-ID",
  "requests": [
    {
      "method": "GET",
      "path": "/api/status",
      "header": {
        "Connection": "keep-alive",
        "X-Test": ["a", "b", "c"]
      }
    }
  ]
}
`)
	var p params
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Error(err)
	}
	if p.Requests[0].Header.Get("Connection") != "keep-alive" {
		t.Fail()
	}
	ss := p.Requests[0].Header.Values("X-Test")
	if ss[0] != "a" || ss[1] != "b" || ss[2] != "c" {
		t.Fail()
	}
}
