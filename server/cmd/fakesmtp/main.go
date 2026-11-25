// fakesmtp — 本地/E2E 用进程内 SMTP 收信服务（仅测试与 CI，不进生产路径）。
//
//	go run ./cmd/fakesmtp --addr :2525 --http :2526
//
// 后端用 EMAIL_PROVIDER=smtp / SMTP_HOST=localhost:2525 / SMTP_USER=... 指向它；
// E2E 用 GET http://localhost:2526/messages 取回验证码邮件。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

type message struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type mailbox struct {
	mu   sync.Mutex
	msgs []message
}

func (m *mailbox) add(msg message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
}

func (m *mailbox) list(to string) []message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]message, 0, len(m.msgs))
	for _, msg := range m.msgs {
		if to == "" || msg.To == to {
			out = append(out, msg)
		}
	}
	return out
}

func (m *mailbox) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = nil
}

func main() {
	smtpAddr := flag.String("addr", ":2525", "SMTP listen address")
	httpAddr := flag.String("http", ":2526", "HTTP message API listen address")
	flag.Parse()

	box := &mailbox{}
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"messages": box.list(r.URL.Query().Get("to"))})
			case http.MethodDelete:
				box.clear()
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		log.Printf("fakesmtp: http on %s", *httpAddr)
		log.Fatal(http.ListenAndServe(*httpAddr, mux))
	}()

	ln, err := net.Listen("tcp", *smtpAddr)
	if err != nil {
		log.Fatalf("fakesmtp: listen: %v", err)
	}
	log.Printf("fakesmtp: smtp on %s", *smtpAddr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalf("fakesmtp: accept: %v", err)
		}
		go handleConn(conn, box)
	}
}

func handleConn(conn net.Conn, box *mailbox) {
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	reply := func(line string) {
		_, _ = fmt.Fprintf(writer, "%s\r\n", line)
		_ = writer.Flush()
	}
	reply("220 fakesmtp ESMTP")

	var rcpts []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			// 必须通告 AUTH：Go smtp.SendMail 配置了 Auth 时会要求该扩展。
			_, _ = fmt.Fprint(writer, "250-fakesmtp\r\n250-AUTH PLAIN LOGIN\r\n250 OK\r\n")
			_ = writer.Flush()
		case strings.HasPrefix(upper, "AUTH"):
			reply("235 2.7.0 Authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			rcpts = nil
			reply("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			rcpts = append(rcpts, strings.Trim(strings.TrimSpace(line[len("RCPT TO:"):]), "<>"))
			reply("250 OK")
		case upper == "DATA":
			reply("354 End data with <CR><LF>.<CR><LF>")
			var raw strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if dataLine == ".\r\n" || dataLine == ".\n" {
					break
				}
				raw.WriteString(dataLine)
			}
			subject, html := splitMessage(raw.String())
			box.add(message{To: strings.Join(rcpts, ","), Subject: subject, HTML: html})
			reply("250 OK")
		case upper == "RSET":
			rcpts = nil
			reply("250 OK")
		case upper == "NOOP":
			reply("250 OK")
		case upper == "QUIT":
			reply("221 Bye")
			return
		default:
			reply("250 OK")
		}
	}
}

func splitMessage(raw string) (subject, html string) {
	header, body, found := strings.Cut(raw, "\r\n\r\n")
	if !found {
		header, body, _ = strings.Cut(raw, "\n\n")
	}
	for _, line := range strings.Split(header, "\n") {
		if strings.HasPrefix(strings.ToLower(line), "subject:") {
			subject = strings.TrimSpace(line[len("subject:"):])
		}
	}
	return subject, body
}
