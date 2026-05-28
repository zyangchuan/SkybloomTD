package redisclient

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrNil = errors.New("redis: nil")

type Client struct {
	addr     string
	username string
	password string
	db       int
	timeout  time.Duration
}

func New(rawURL string) (*Client, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	addr := parsed.Host
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, "6379")
	}
	username := parsed.User.Username()
	password, _ := parsed.User.Password()
	db := 0
	if path := strings.Trim(parsed.Path, "/"); path != "" {
		parsedDB, err := strconv.Atoi(path)
		if err != nil {
			return nil, fmt.Errorf("parse redis db: %w", err)
		}
		db = parsedDB
	}
	return &Client{
		addr:     addr,
		username: username,
		password: password,
		db:       db,
		timeout:  5 * time.Second,
	}, nil
}

func (c *Client) Do(args ...string) (any, error) {
	conn, reader, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := writeCommand(conn, args...); err != nil {
		return nil, err
	}
	return readValue(reader)
}

func (c *Client) Close() error {
	return nil
}

func (c *Client) connect() (net.Conn, *bufio.Reader, error) {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return nil, nil, err
	}
	reader := bufio.NewReader(conn)
	if c.password != "" {
		args := []string{"AUTH", c.password}
		if c.username != "" {
			args = []string{"AUTH", c.username, c.password}
		}
		if err := writeCommand(conn, args...); err != nil {
			conn.Close()
			return nil, nil, err
		}
		if _, err := readValue(reader); err != nil {
			conn.Close()
			return nil, nil, err
		}
	}
	if c.db != 0 {
		if err := writeCommand(conn, "SELECT", strconv.Itoa(c.db)); err != nil {
			conn.Close()
			return nil, nil, err
		}
		if _, err := readValue(reader); err != nil {
			conn.Close()
			return nil, nil, err
		}
	}
	return conn, reader, nil
}

func writeCommand(writer io.Writer, args ...string) error {
	if _, err := fmt.Fprintf(writer, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(writer, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

func readValue(reader *bufio.Reader) (any, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		return readLine(reader)
	case '-':
		message, _ := readLine(reader)
		return nil, errors.New(message)
	case ':':
		raw, err := readLine(reader)
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(raw, 10, 64)
	case '$':
		return readBulkString(reader)
	case '*':
		return readArray(reader)
	default:
		return nil, fmt.Errorf("unsupported redis response prefix %q", prefix)
	}
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func readBulkString(reader *bufio.Reader) (string, error) {
	rawLength, err := readLine(reader)
	if err != nil {
		return "", err
	}
	length, err := strconv.Atoi(rawLength)
	if err != nil {
		return "", err
	}
	if length == -1 {
		return "", ErrNil
	}
	value := make([]byte, length+2)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return string(value[:length]), nil
}

func readArray(reader *bufio.Reader) ([]any, error) {
	rawLength, err := readLine(reader)
	if err != nil {
		return nil, err
	}
	length, err := strconv.Atoi(rawLength)
	if err != nil {
		return nil, err
	}
	if length == -1 {
		return nil, ErrNil
	}
	values := make([]any, 0, length)
	for i := 0; i < length; i++ {
		value, err := readValue(reader)
		if errors.Is(err, ErrNil) {
			values = append(values, nil)
			continue
		}
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
