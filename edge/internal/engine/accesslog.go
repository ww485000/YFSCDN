package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AccessLogEntry is one access-log record (also uploaded to core).
type AccessLogEntry struct {
	TS      string `json:"ts"`
	SiteID  int64  `json:"site_id"`
	IP      string `json:"ip"`
	Host    string `json:"host"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Proto   string `json:"proto"`
	Status  int    `json:"status"`
	Bytes   int64  `json:"bytes"`
	Referer string `json:"referer"`
	UA      string `json:"ua"`
	Latency int64  `json:"latency_ms"`
	Cache   string `json:"cache"` // HIT | MISS | STALE | BYPASS
}

// AccessLogWriter has two independent consumers:
//  1. a local file (nginx-style lines) fed through a channel;
//  2. an in-memory buffer drained by Take() for upload to core.
// The file path never steals entries from the upload path.
type AccessLogWriter struct {
	dir    string
	mu     sync.Mutex
	buf    []AccessLogEntry
	fl     *os.File
	fw     *bufio.Writer
	fileCh chan *AccessLogEntry
	stop   chan struct{}
	once   sync.Once
}

// NewAccessLogWriter creates the writer. dir = local log dir ("" disables file).
func NewAccessLogWriter(dir string) *AccessLogWriter {
	aw := &AccessLogWriter{
		dir:    dir,
		buf:    []AccessLogEntry{},
		fileCh: make(chan *AccessLogEntry, 20000),
		stop:   make(chan struct{}),
	}
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		if f, err := os.OpenFile(filepath.Join(dir, "access.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			aw.fl = f
			aw.fw = bufio.NewWriterSize(f, 256*1024)
			go aw.fileLoop()
		}
	}
	return aw
}

// Record appends one entry (non-blocking, drops the upload copy when the
// buffer is huge).
func (aw *AccessLogWriter) Record(e AccessLogEntry) {
	if aw == nil {
		return
	}
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if aw.fileCh != nil {
		select {
		case aw.fileCh <- &e:
		default: // file queue full; drop the file copy only
		}
	}
	aw.mu.Lock()
	if len(aw.buf) > 50000 {
		aw.mu.Unlock()
		return
	}
	aw.buf = append(aw.buf, e)
	aw.mu.Unlock()
}

// Take drains up to max entries (as maps) for upload to core.
func (aw *AccessLogWriter) Take(max int) []map[string]any {
	if aw == nil {
		return nil
	}
	aw.mu.Lock()
	if max <= 0 || max > len(aw.buf) {
		max = len(aw.buf)
	}
	if max == 0 {
		aw.mu.Unlock()
		return nil
	}
	batch := aw.buf[:max]
	aw.buf = append([]AccessLogEntry(nil), aw.buf[max:]...)
	aw.mu.Unlock()

	out := make([]map[string]any, 0, len(batch))
	for i := range batch {
		e := batch[i]
		out = append(out, map[string]any{
			"ts": e.TS, "site_id": e.SiteID, "ip": e.IP, "host": e.Host,
			"method": e.Method, "path": e.Path, "proto": e.Proto,
			"status": e.Status, "bytes": e.Bytes,
			"referer": e.Referer, "ua": e.UA, "latency_ms": e.Latency, "cache": e.Cache,
		})
	}
	return out
}

// fileLoop writes entries to the local file; flushes periodically.
func (aw *AccessLogWriter) fileLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	written := 0
	for {
		select {
		case <-aw.stop:
			// drain remainder
			for {
				select {
				case e := <-aw.fileCh:
					aw.writeLine(e)
				default:
					aw.flushFile()
					if aw.fl != nil {
						_ = aw.fl.Close()
					}
					return
				}
			}
		case e := <-aw.fileCh:
			aw.writeLine(e)
			written++
			if written >= 128 {
				aw.flushFile()
				written = 0
			}
		case <-ticker.C:
			aw.flushFile()
		}
	}
}

func (aw *AccessLogWriter) writeLine(e *AccessLogEntry) {
	if aw.fw == nil {
		return
	}
	_, _ = aw.fw.WriteString(aw.render(e))
}

func (aw *AccessLogWriter) flushFile() {
	if aw.fw != nil {
		_ = aw.fw.Flush()
	}
}

func (aw *AccessLogWriter) render(e *AccessLogEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %d %s %s %s %s %s %d %d %d %q %q %s\n",
		e.TS, e.SiteID, e.Host, e.IP, e.Method, e.Path, e.Proto,
		e.Status, e.Bytes, e.Latency, e.Referer, e.UA, e.Cache)
	return b.String()
}

// Close stops the file loop (the loop closes the file after draining).
func (aw *AccessLogWriter) Close() {
	if aw == nil {
		return
	}
	aw.once.Do(func() {
		close(aw.stop)
	})
}
