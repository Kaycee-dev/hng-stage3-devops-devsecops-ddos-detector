package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"
)

type AccessLog struct {
	SourceIP     string    `json:"source_ip"`
	Timestamp    string    `json:"timestamp"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	Status       int       `json:"status"`
	ResponseSize int64     `json:"response_size"`
	ParsedTime   time.Time `json:"-"`
}

type rawAccessLog struct {
	SourceIP     string      `json:"source_ip"`
	Timestamp    string      `json:"timestamp"`
	Method       string      `json:"method"`
	Path         string      `json:"path"`
	Status       interface{} `json:"status"`
	ResponseSize interface{} `json:"response_size"`
}

type LogMonitor struct {
	Path        string
	TailFromEnd bool
	PollEvery   time.Duration
	Logger      *log.Logger
}

func ParseAccessLogLine(line []byte) (AccessLog, error) {
	var raw rawAccessLog
	if err := json.Unmarshal(line, &raw); err != nil {
		return AccessLog{}, err
	}
	if raw.SourceIP == "" || raw.Timestamp == "" || raw.Method == "" || raw.Path == "" {
		return AccessLog{}, errors.New("missing required access log fields")
	}
	status, err := numberAsInt(raw.Status)
	if err != nil {
		return AccessLog{}, fmt.Errorf("invalid status: %w", err)
	}
	size, err := numberAsInt64(raw.ResponseSize)
	if err != nil {
		return AccessLog{}, fmt.Errorf("invalid response_size: %w", err)
	}
	parsedTime, err := parseAccessLogTimestamp(raw.Timestamp)
	if err != nil {
		return AccessLog{}, err
	}
	return AccessLog{
		SourceIP:     raw.SourceIP,
		Timestamp:    raw.Timestamp,
		Method:       raw.Method,
		Path:         raw.Path,
		Status:       status,
		ResponseSize: size,
		ParsedTime:   parsedTime,
	}, nil
}

func parseAccessLogTimestamp(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"02/Jan/2006:15:04:05 -0700",
	}
	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q: %w", value, lastErr)
}

func numberAsInt(value interface{}) (int, error) {
	parsed, err := numberAsInt64(value)
	return int(parsed), err
}

func numberAsInt64(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported number type %T", value)
	}
}

func (m LogMonitor) Tail(ctx context.Context, out chan<- AccessLog) error {
	pollEvery := m.PollEvery
	if pollEvery <= 0 {
		pollEvery = 250 * time.Millisecond
	}
	logger := m.Logger
	if logger == nil {
		logger = log.Default()
	}

	var offset int64
	initialized := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		file, err := os.Open(m.Path)
		if err != nil {
			logger.Printf("waiting for access log %s: %v", m.Path, err)
			if !sleepContext(ctx, time.Second) {
				return ctx.Err()
			}
			continue
		}

		info, err := file.Stat()
		if err != nil {
			file.Close()
			return err
		}
		if !initialized && m.TailFromEnd {
			offset = info.Size()
		}
		if info.Size() < offset {
			offset = 0
		}
		initialized = true

		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			file.Close()
			return err
		}
		reader := bufio.NewReader(file)

		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				offset += int64(len(line))
				entry, parseErr := ParseAccessLogLine(line)
				if parseErr != nil {
					logger.Printf("skipping malformed access log line: %v", parseErr)
				} else {
					select {
					case out <- entry:
					case <-ctx.Done():
						file.Close()
						return ctx.Err()
					}
				}
			}
			if err == nil {
				continue
			}
			if !errors.Is(err, io.EOF) {
				file.Close()
				return err
			}
			info, statErr := file.Stat()
			if statErr != nil {
				file.Close()
				return statErr
			}
			if info.Size() < offset {
				file.Close()
				break
			}
			if !sleepContext(ctx, pollEvery) {
				file.Close()
				return ctx.Err()
			}
		}
	}
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
