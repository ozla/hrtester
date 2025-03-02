package tester

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/ozla/hrtester/internal/log"
	"github.com/ozla/hrtester/internal/shared"
)

////////////////////////////////////////////////////////////////////////////////

type params struct {
	Name            string          `json:"name"`
	Duration        shared.Duration `json:"duration"`
	Pace            pace            `json:"pace"`
	ParallelTesters uint8           `json:"parallelTesters"`
	Timeout         shared.Duration `json:"timeout"`
	Choice          choice          `json:"choice"`
	ReqSchema       schema          `json:"reqSchema"`
	ReqVersion      version         `json:"reqVersion"`
	ReqIDHeader     string          `json:"reqIDHeader"`
	Requests        []request       `json:"requests"`
}

func (p *params) UnmarshalJSON(data []byte) error {
	type alias params

	aux := struct {
		alias
		Requests []json.RawMessage `json:"requests"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*p = params(aux.alias)
	p.Requests = make([]request, len(aux.Requests))
	for i, raw := range aux.Requests {
		if err := json.Unmarshal(raw, &p.Requests[i]); err != nil {
			log.Debug(
				"error unmarshaling request object",
				slog.Any("err", err),
				slog.String("value", string(raw)),
			)
			return fmt.Errorf("invalid request at index %d: %v", i, err)
		}
	}

	return nil
}

////////////////////////////////////////////////////////////////////////////////

type request struct {
	Method method      `json:"method"`
	Path   string      `json:"path"`
	Header http.Header `json:"header"`
	Body   string      `json:"body"`
}

func (r *request) UnmarshalJSON(data []byte) error {
	type alias request

	aux := struct {
		alias
		Header map[string]json.RawMessage `json:"header"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*r = request(aux.alias)
	r.Header = make(http.Header, len(aux.Header))
	for k, raw := range aux.Header {
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("invalid header %s value: %v", k, err)
		}
		switch v := parsed.(type) {
		case string:
			r.Header.Set(k, v)
		case []any:
			for _, v := range v {
				if s, ok := v.(string); ok {
					r.Header.Add(k, s)
				} else {
					return fmt.Errorf("invalid header %s value", k)
				}
			}
		}
	}

	return nil
}

////////////////////////////////////////////////////////////////////////////////

type schema string

func (s schema) MarshalJSON() ([]byte, error) {
	switch s {
	case "http", "https":
		return []byte(`"` + string(s) + `"`), nil
	default:
		return nil, fmt.Errorf("invalid choice value: must be 'http' or 'https'")
	}
}

func (s *schema) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON string: %v", err)
	}

	switch v {
	case "http", "https":
		*s = schema(v)
		return nil
	default:
		return fmt.Errorf("invalid choice value: must be 'http' or 'https'")
	}
}

////////////////////////////////////////////////////////////////////////////////

type choice string

func (c choice) MarshalJSON() ([]byte, error) {
	switch c {
	case "roundrobin", "random":
		return []byte(`"` + string(c) + `"`), nil
	default:
		return nil, fmt.Errorf("invalid choice value: must be 'roundrobin' or 'random'")
	}
}

func (c *choice) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid JSON string: %v", err)
	}

	switch s {
	case "roundrobin", "random":
		*c = choice(s)
		return nil
	default:
		return fmt.Errorf("invalid choice value: must be 'roundrobin' or 'random'")
	}
}

////////////////////////////////////////////////////////////////////////////////

type method string

var validMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodPatch:   true,
	http.MethodDelete:  true,
	http.MethodConnect: true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

func (m method) MarshalJSON() ([]byte, error) {
	if _, ok := validMethods[string(m)]; !ok {
		return nil, fmt.Errorf("invalid HTTP method: '%s'", m)
	}
	return []byte(`"` + string(m) + `"`), nil
}

func (m *method) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid JSON string: %v", err)
	}

	if _, ok := validMethods[s]; !ok {
		return fmt.Errorf("invalid HTTP method: '%s'", s)
	}

	*m = method(s)
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type version [2]uint8

func (v version) MarshalJSON() ([]byte, error) {
	s := `"` + strconv.Itoa(int(v[0])) + "." + strconv.Itoa(int(v[1])) + `"`
	return []byte(s), nil
}

func (v *version) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid JSON string: %v", err)
	}

	fields := strings.Split(s, ".")
	if len(fields) != 2 {
		return fmt.Errorf("invalid version format: expected 'major.minor', got '%s'", s)
	}

	major, err := strconv.ParseUint(fields[0], 10, 8)
	if err != nil {
		return fmt.Errorf("invalid version major value: '%s'", s)
	}
	minor, err := strconv.ParseUint(fields[1], 10, 8)
	if err != nil {
		return fmt.Errorf("invalid version minor value: '%s'", s)
	}

	*v = [2]uint8{uint8(major), uint8(minor)}

	return nil
}

////////////////////////////////////////////////////////////////////////////////

type pace uint16

func (p pace) MarshalJSON() ([]byte, error) {
	s := `"` + strconv.Itoa(int(p)) + `rpm"`
	return []byte(s), nil
}

func (p *pace) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid JSON string: %v", err)
	}

	allowed := []string{"rps", "rpm", "rph"}
	valid := false
	for _, unit := range allowed {
		if strings.HasSuffix(s, unit) {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid duration format: must end with 'rps', 'rpm', or 'rph'")
	}

	switch v, u := s[:len(s)-3], s[len(s)-3:]; u {
	case "rph":
		if v, err := strconv.ParseUint(v, 10, 16); err == nil {
			*p = pace(math.Round(float64(v) / 60))
		} else {
			return fmt.Errorf("invalid pace value: '%s'", s)
		}
	case "rpm":
		if v, err := strconv.ParseUint(v, 10, 16); err == nil {
			*p = pace(v)
		} else {
			return fmt.Errorf("invalid pace value: '%s'", s)
		}
	case "rps":
		if v, err := strconv.ParseUint(v, 10, 16); err == nil {
			*p = pace(v * 60)
		} else {
			return fmt.Errorf("invalid pace value: '%s'", s)
		}
	}

	return nil
}

func (p pace) String() string {
	return strconv.FormatUint(uint64(p), 10) + "rpm"
}

////////////////////////////////////////////////////////////////////////////////
