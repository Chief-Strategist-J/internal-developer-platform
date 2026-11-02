package adapters

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// RedisRevocationAdapter connects directly to Redis using RESP protocol (zero external deps)
type RedisRevocationAdapter struct {
	addr    string
	timeout time.Duration
	mu      sync.Mutex
}

func NewRedisRevocationAdapter(addr string, timeout time.Duration) *RedisRevocationAdapter {
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &RedisRevocationAdapter{
		addr:    addr,
		timeout: timeout,
	}
}

func (a *RedisRevocationAdapter) sendCommand(cmd string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	conn, err := net.DialTimeout("tcp", a.addr, a.timeout)
	if err != nil {
		return "", fmt.Errorf("redis dial failed: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(a.timeout))

	if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
		return "", fmt.Errorf("redis write failed: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("redis read failed: %w", err)
	}

	return strings.TrimSpace(line), nil
}

func (a *RedisRevocationAdapter) IsRevoked(ctx context.Context, subjectID string, jti string) (bool, error) {
	if subjectID != "" {
		resp, err := a.sendCommand(fmt.Sprintf("EXISTS revocation:sub:%s", subjectID))
		if err == nil && resp == ":1" {
			return true, nil
		}
	}

	if jti != "" {
		resp, err := a.sendCommand(fmt.Sprintf("EXISTS revocation:jti:%s", jti))
		if err == nil && resp == ":1" {
			return true, nil
		}
	}

	return false, nil
}

func (a *RedisRevocationAdapter) RevokeSubject(ctx context.Context, subjectID string) error {
	resp, err := a.sendCommand(fmt.Sprintf("SETEX revocation:sub:%s 86400 revoked", subjectID))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "+OK") {
		return fmt.Errorf("unexpected redis response: %s", resp)
	}
	return nil
}

func (a *RedisRevocationAdapter) RevokeJTI(ctx context.Context, jti string) error {
	resp, err := a.sendCommand(fmt.Sprintf("SETEX revocation:jti:%s 86400 revoked", jti))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "+OK") {
		return fmt.Errorf("unexpected redis response: %s", resp)
	}
	return nil
}
