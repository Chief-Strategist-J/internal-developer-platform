package adapters

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	rbacQueries "internal-developer-platform/src/features/role-based-access-control/infrastructure/queries"
)

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
		cmd := rbacQueries.BuildExistsCommand(rbacQueries.BuildRevocationSubKey(subjectID))
		resp, err := a.sendCommand(cmd)
		if err == nil && resp == rbacQueries.ExistsPositiveResult {
			return true, nil
		}
	}

	if jti != "" {
		cmd := rbacQueries.BuildExistsCommand(rbacQueries.BuildRevocationJTIKey(jti))
		resp, err := a.sendCommand(cmd)
		if err == nil && resp == rbacQueries.ExistsPositiveResult {
			return true, nil
		}
	}

	return false, nil
}

func (a *RedisRevocationAdapter) RevokeSubject(ctx context.Context, subjectID string) error {
	cmd := rbacQueries.BuildSetexCommand(
		rbacQueries.BuildRevocationSubKey(subjectID),
		rbacQueries.DefaultTTLSeconds,
		rbacQueries.RevokedSentinelValue,
	)
	resp, err := a.sendCommand(cmd)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, rbacQueries.OkResponsePrefix) {
		return fmt.Errorf("unexpected redis response: %s", resp)
	}
	return nil
}

func (a *RedisRevocationAdapter) RevokeJTI(ctx context.Context, jti string) error {
	cmd := rbacQueries.BuildSetexCommand(
		rbacQueries.BuildRevocationJTIKey(jti),
		rbacQueries.DefaultTTLSeconds,
		rbacQueries.RevokedSentinelValue,
	)
	resp, err := a.sendCommand(cmd)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, rbacQueries.OkResponsePrefix) {
		return fmt.Errorf("unexpected redis response: %s", resp)
	}
	return nil
}
