package capture

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
	"github.com/eebustracer/eebustracer/internal/parser"
)

// TruncatedReporter is implemented by anything that wants to count DLT
// frames dropped mid-payload (EEBus-shaped but with truncated JSON). The
// capture Engine implements it and forwards the count into the trace's
// SkippedTruncated field so users see "N truncated" in the header.
type TruncatedReporter interface {
	IncTruncated()
}

// DLTStreamSource connects to a dlt-daemon TCP listener (AUTOSAR default
// port 3490) and reads standard-header-framed DLT messages. Verbose string
// payloads matching an EEBus extractor (Porsche CEM patterns or the generic
// SHIP/SPINE prefix scan) are decoded and emitted; everything else is
// silently dropped so signal/noise stays high on chatty ECU streams.
//
// The source reconnects with exponential backoff when the connection drops
// (ECU reboot, dlt-daemon restart, transient network loss) until the
// enclosing context is cancelled.
type DLTStreamSource struct {
	target   string
	filter   DLTFilter
	parser   *parser.Parser
	logger   *slog.Logger
	reporter TruncatedReporter // optional — nil means "don't count"
	seqNum   int
	dialFunc func(ctx context.Context, target string) (net.Conn, error) // test hook
}

// NewDLTStreamSource creates a DLT stream source. filter may be empty (accept
// all APID/CTID combinations) or a comma-separated list of `APID` or
// `APID:CTID` entries — see ParseDLTFilter.
func NewDLTStreamSource(target string, filter DLTFilter, p *parser.Parser, logger *slog.Logger) *DLTStreamSource {
	return &DLTStreamSource{
		target: target,
		filter: filter,
		parser: p,
		logger: logger,
	}
}

// SetTruncatedReporter installs a reporter that will be notified whenever a
// DLT frame carrying EEBus-shaped content is dropped because its payload was
// truncated. Optional — leaving it unset just means truncations aren't
// counted (they're still logged at debug level).
func (s *DLTStreamSource) SetTruncatedReporter(r TruncatedReporter) {
	s.reporter = r
}

// Name returns "dlt".
func (s *DLTStreamSource) Name() string { return "dlt" }

// Run dials the dlt-daemon and reads DLT frames until the context is
// cancelled. Reconnects with exponential backoff on connection failure.
func (s *DLTStreamSource) Run(ctx context.Context, emit func(*model.Message)) error {
	dial := s.dialFunc
	if dial == nil {
		dial = defaultDLTDial
	}
	backoff := time.Second
	const backoffMax = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return nil
		}

		conn, err := dial(ctx, s.target)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Warn("dlt: connect failed, retrying",
				"target", s.target,
				"error", err,
				"backoff", backoff,
			)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
			continue
		}
		backoff = time.Second // reset on successful connect
		s.logger.Info("dlt: connected", "target", s.target)

		s.readUntilClosed(ctx, conn, emit)
		conn.Close()

		if ctx.Err() != nil {
			return nil
		}
		s.logger.Info("dlt: connection lost, reconnecting", "target", s.target)
	}
}

// readUntilClosed reads DLT frames from conn until EOF or context cancel.
// Runs one connection's lifetime; the outer Run loop handles reconnect.
func (s *DLTStreamSource) readUntilClosed(ctx context.Context, conn net.Conn, emit func(*model.Message)) {
	// Close the connection when context is cancelled to unblock any read.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	for {
		frame, err := parser.ReadDLTMessageStream(reader, time.Now().UTC())
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Warn("dlt: read frame failed", "error", err)
			return
		}

		if !s.filter.Accept(frame.APID, frame.CTID) {
			continue
		}

		s.seqNum++
		msg, truncated, err := parser.DLTFrameToMessage(s.parser, frame, s.seqNum)
		if err != nil {
			s.logger.Warn("dlt: message build failed", "error", err)
			continue
		}
		if truncated {
			if s.reporter != nil {
				s.reporter.IncTruncated()
			}
			s.logger.Debug("dlt: dropped truncated EEBus frame",
				"apid", frame.APID,
				"ctid", frame.CTID,
			)
			continue
		}
		if msg == nil {
			continue
		}
		emit(msg)
	}
}

func defaultDLTDial(ctx context.Context, target string) (net.Conn, error) {
	var d net.Dialer
	d.Timeout = 10 * time.Second
	return d.DialContext(ctx, "tcp", target)
}

// DLTFilter accepts or rejects DLT frames by APID and/or CTID. Empty filter
// accepts everything. Each rule matches on APID (required) and optionally
// CTID (empty = wildcard). Match is OR across all rules.
type DLTFilter struct {
	rules []dltFilterRule
}

type dltFilterRule struct {
	apid string
	ctid string // empty = any CTID
}

// Accept reports whether the given APID/CTID pair passes the filter.
func (f DLTFilter) Accept(apid, ctid string) bool {
	if len(f.rules) == 0 {
		return true
	}
	for _, r := range f.rules {
		if r.apid != apid {
			continue
		}
		if r.ctid == "" || r.ctid == ctid {
			return true
		}
	}
	return false
}

// IsEmpty reports whether no filter rules are configured.
func (f DLTFilter) IsEmpty() bool { return len(f.rules) == 0 }

// String renders the filter back to its canonical `APID[:CTID],…` form.
func (f DLTFilter) String() string {
	if len(f.rules) == 0 {
		return ""
	}
	parts := make([]string, len(f.rules))
	for i, r := range f.rules {
		if r.ctid == "" {
			parts[i] = r.apid
		} else {
			parts[i] = r.apid + ":" + r.ctid
		}
	}
	return strings.Join(parts, ",")
}

// ParseDLTFilter parses a comma-separated filter string into a DLTFilter.
// Each entry is either `APID` (accept any CTID for that APID) or `APID:CTID`
// (exact match). Whitespace and empty entries are ignored. An empty input
// returns an empty filter (accept-all).
//
// Examples:
//
//	""                 → accept all
//	"CEM"              → accept any CEM/*
//	"CEM,HEMS:HEMS"    → accept CEM/* or HEMS/HEMS
func ParseDLTFilter(s string) (DLTFilter, error) {
	var f DLTFilter
	s = strings.TrimSpace(s)
	if s == "" {
		return f, nil
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.IndexByte(part, ':')
		var r dltFilterRule
		if colon < 0 {
			r.apid = part
		} else {
			r.apid = strings.TrimSpace(part[:colon])
			r.ctid = strings.TrimSpace(part[colon+1:])
		}
		if r.apid == "" {
			return DLTFilter{}, fmt.Errorf("dlt filter: empty APID in %q", part)
		}
		if len(r.apid) > 4 || len(r.ctid) > 4 {
			return DLTFilter{}, fmt.Errorf("dlt filter: APID/CTID must be ≤4 chars in %q", part)
		}
		f.rules = append(f.rules, r)
	}
	return f, nil
}
