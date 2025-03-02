package shared

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

////////////////////////////////////////////////////////////////////////////////

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	if d == 0 {
		return []byte("null"), nil
	}
	ms := strconv.FormatInt(time.Duration(d).Milliseconds(), 10) + "ms"
	return json.Marshal(ms)
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid JSON string: %v", err)
	}

	allowed := []string{"ms", "s", "m"}
	valid := false
	for _, unit := range allowed {
		if strings.HasSuffix(s, unit) {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid duration format: must end with 'ms', 's', or 'm'")
	}

	value, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration value: %v", err)
	}

	*d = Duration(value)
	return nil
}

func (d Duration) String() string {
	return strconv.FormatInt(time.Duration(d).Milliseconds(), 10) + "ms"
}

////////////////////////////////////////////////////////////////////////////////

var attrNames = [...]string{
	"ReqTime",
	"TestName",
	"ReqID",
	"ReqNum",
	"ReqMethod",
	"ReqPath",
	"RespCode",
	"RoundDuration",
	"TimedOut",
}

const (
	trRequestTime = iota
	trTestName
	trRequestID
	trRequestNum
	trRequestMethod
	trRequestPath
	trResponseCode
	trRoundDuration
	trTimedOut
)

type TestResult [len(attrNames)]string

func NewTestResult(vs url.Values) TestResult {
	var r TestResult
	for i, n := range attrNames {
		r[i] = vs.Get(n)
	}
	return r
}

func (r *TestResult) SetRequestTime(t time.Time) {
	r[trRequestTime] = t.Format(time.DateTime)
}

func (r *TestResult) SetTestName(name string) {
	r[trTestName] = name
}

func (r *TestResult) SetRequestID(id uuid.UUID) {
	r[trRequestID] = id.String()
}

func (r *TestResult) SetRequestNum(num uint64) {
	r[trRequestNum] = strconv.FormatInt(int64(num), 10)
}

func (r *TestResult) SetRequesMethod(method string) {
	r[trRequestMethod] = method
}

func (r *TestResult) SetRequestPath(path string) {
	r[trRequestPath] = path
}

func (r *TestResult) SetResponseCode(code int) {
	r[trResponseCode] = strconv.Itoa(code)
}

func (r *TestResult) SetRoundDuration(d Duration) {
	r[trRoundDuration] = d.String()
}

func (r *TestResult) SetTimedOut(v bool) {
	if v {
		r[trTimedOut] = "true"
	} else {
		r[trTimedOut] = "false"
	}
}

func (r TestResult) URLValues() url.Values {
	m := make(url.Values)
	for i, v := range attrNames {
		m.Set(v, r[i])
	}
	return m
}

func (r TestResult) Slice() []string {
	return r[:]
}

////////////////////////////////////////////////////////////////////////////////
