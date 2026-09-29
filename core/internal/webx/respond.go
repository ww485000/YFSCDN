package webx

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Response is the unified API envelope: {code, message, data}.
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// PageData wraps list results.
type PageData struct {
	List  any   `json:"list"`
	Total int64 `json:"total"`
}

// OK writes {code:0, message:"ok", data}.
func OK(w http.ResponseWriter, data any) {
	write(w, http.StatusOK, 0, "ok", data)
}

// Page writes a list result with total.
func Page(w http.ResponseWriter, list any, total int64) {
	OK(w, PageData{List: list, Total: total})
}

// Fail writes an error envelope with the given HTTP status.
func Fail(w http.ResponseWriter, status, code int, msg string) {
	write(w, status, code, msg, nil)
}

func write(w http.ResponseWriter, status, code int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{Code: code, Message: msg, Data: data})
}

// BindJSON decodes the request body into v; returns error for bad JSON.
func BindJSON(c *Context, v any) error {
	defer c.R.Body.Close()
	return json.NewDecoder(c.R.Body).Decode(v)
}

// ParamInt64 parses a numeric path parameter (":id").
func ParamInt64(c *Context, name string) (int64, error) {
	s := c.Params[name]
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("bad id parameter")
	}
	return v, nil
}

// QueryInt reads an integer query param with a default.
func QueryInt(c *Context, name string, def int) int {
	v := c.R.URL.Query().Get(name)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

// QueryInt64 reads an int64 query param with a default.
func QueryInt64(c *Context, name string, def int64) int64 {
	v := c.R.URL.Query().Get(name)
	if v == "" {
		return def
	}
	var n int64
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

// QueryStr reads a string query param.
func QueryStr(c *Context, name string) string {
	return c.R.URL.Query().Get(name)
}
