package operator

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"time"
)

const auditEmailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Audit Report</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
                   "Helvetica Neue", Arial, sans-serif;
      background: #f4f6f9;
      color: #1a1d23;
      padding: 32px 16px;
    }

    .wrapper {
      max-width: 900px;
      margin: 0 auto;
    }

    .header {
      background: #0f1623;
      border-radius: 8px 8px 0 0;
      padding: 28px 36px;
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .header-brand {
      font-size: 20px;
      font-weight: 700;
      color: #ffffff;
      letter-spacing: 0.5px;
    }
    .header-brand span {
      color: #4f8ef7;
    }
    .header-meta {
      text-align: right;
      color: #8a93a8;
      font-size: 13px;
      line-height: 1.6;
    }
    .header-meta strong {
      color: #c8cdd9;
    }

    .summary {
      background: #ffffff;
      border-left: 1px solid #e2e6ed;
      border-right: 1px solid #e2e6ed;
      padding: 20px 36px;
      display: flex;
      gap: 32px;
      flex-wrap: wrap;
    }
    .stat {
      display: flex;
      flex-direction: column;
      gap: 2px;
    }
    .stat-value {
      font-size: 26px;
      font-weight: 700;
      line-height: 1;
    }
    .stat-label {
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.6px;
      color: #6b7280;
    }
    .stat-value.updated  { color: #2563eb; }
    .stat-value.skipped  { color: #6b7280; }
    .stat-value.errors   { color: #dc2626; }

    .table-wrap {
      background: #ffffff;
      border: 1px solid #e2e6ed;
      border-top: none;
      overflow-x: auto;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 13px;
    }
    thead tr {
      background: #f8fafc;
      border-bottom: 2px solid #e2e6ed;
    }
    th {
      padding: 11px 14px;
      text-align: left;
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.6px;
      color: #6b7280;
      white-space: nowrap;
    }
    tbody tr {
      border-bottom: 1px solid #f0f2f5;
      transition: background 0.1s;
    }
    tbody tr:last-child { border-bottom: none; }
    tbody tr:hover { background: #f8fafc; }
    td {
      padding: 10px 14px;
      vertical-align: top;
      color: #374151;
    }
    td.mono {
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
      font-size: 12px;
      color: #4b5563;
    }

    .badge {
      display: inline-block;
      padding: 2px 8px;
      border-radius: 9999px;
      font-size: 11px;
      font-weight: 600;
      letter-spacing: 0.3px;
      white-space: nowrap;
    }
    .badge-update   { background: #dbeafe; color: #1d4ed8; }
    .badge-skip     { background: #f3f4f6; color: #6b7280; }
    .badge-error    { background: #fee2e2; color: #b91c1c; }
    .badge-added    { background: #dcfce7; color: #15803d; }
    .badge-removed  { background: #fee2e2; color: #b91c1c; }
    .badge-modified { background: #fef9c3; color: #854d0e; }

    .changes { display: flex; flex-direction: column; gap: 4px; }
    .change-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
    .change-field {
      font-family: "SFMono-Regular", Consolas, monospace;
      font-size: 11px;
      color: #374151;
    }
    .change-arrow { color: #9ca3af; font-size: 11px; }
    .change-val {
      font-family: "SFMono-Regular", Consolas, monospace;
      font-size: 11px;
    }
    .val-old { color: #b91c1c; text-decoration: line-through; }
    .val-new { color: #15803d; }

    .error-text {
      color: #b91c1c;
      font-size: 12px;
      font-family: "SFMono-Regular", Consolas, monospace;
    }

    .footer {
      background: #f8fafc;
      border: 1px solid #e2e6ed;
      border-top: none;
      border-radius: 0 0 8px 8px;
      padding: 16px 36px;
      font-size: 12px;
      color: #9ca3af;
      display: flex;
      justify-content: space-between;
      flex-wrap: wrap;
      gap: 8px;
    }
  </style>
</head>
<body>
<div class="wrapper">

  <div class="header">
    <div class="header-meta">
      <strong>Audit Report</strong><br />
      Target: <strong>{{ .TargetName }}</strong><br />
      Generated: {{ .GeneratedAt }}
    </div>
  </div>

  <div class="summary">
    <div class="stat">
      <span class="stat-value updated">{{ .Counts.Updated }}</span>
      <span class="stat-label">Updated</span>
    </div>
    <div class="stat">
      <span class="stat-value skipped">{{ .Counts.Skipped }}</span>
      <span class="stat-label">Skipped</span>
    </div>
    <div class="stat">
      <span class="stat-value errors">{{ .Counts.Errors }}</span>
      <span class="stat-label">Errors</span>
    </div>
    <div class="stat">
      <span class="stat-value" style="color:#374151;">{{ .Counts.Total }}</span>
      <span class="stat-label">Total entries</span>
    </div>
  </div>

  <div class="table-wrap">
    <table>
      <thead>
        <tr>
          <th>Timestamp</th>
          <th>Action</th>
          <th>UID</th>
          <th>Name</th>
          <th>Changes</th>
          <th>Error</th>
        </tr>
      </thead>
      <tbody>
        {{ range .Entries }}
        <tr>
          <td class="mono">{{ .TimestampFmt }}</td>
          <td>{{ actionBadge .Action }}</td>
          <td class="mono">{{ .UID }}</td>
          <td>{{ .Name }}</td>
          <td>
            {{ if .Changes }}
            <div class="changes">
              {{ range .Changes }}
              <div class="change-row">
                {{ kindBadge .Diff }}
                <span class="change-field">{{ .Field }}</span>
                {{ if .Old }}<span class="change-val val-old">{{ .Old }}</span>{{ end }}
                {{ if and .Old .New }}<span class="change-arrow">→</span>{{ end }}
                {{ if .New }}<span class="change-val val-new">{{ .New }}</span>{{ end }}
              </div>
              {{ end }}
            </div>
            {{ end }}
          </td>
          <td>
            {{ if .ErrorStr }}
            <span class="error-text">{{ .ErrorStr }}</span>
            {{ end }}
          </td>
        </tr>
        {{ end }}
      </tbody>
    </table>
  </div>

  <div class="footer">
    <span>Report ID: {{ .ReportID }}</span>
  </div>

</div>
</body>
</html>`

type auditEmailData struct {
	TargetName  string
	GeneratedAt string
	ReportID    string
	Counts      auditCounts
	Entries     []auditEmailEntry
}

type auditCounts struct {
	Updated int
	Skipped int
	Errors  int
	Total   int
}

type auditEmailEntry struct {
	TimestampFmt string
	Action       Action
	UID          string
	Name         string
	Changes      []auditEmailChange
	ErrorStr     string
}

type auditEmailChange struct {
	Diff  string
	Field string
	Old   string
	New   string
}

var emailTmpl = template.Must(
	template.New("audit").Funcs(template.FuncMap{
		"actionBadge": func(a Action) template.HTML {
			switch a {
			case ActionUpdate:
				return `<span class="badge badge-update">UPDATE</span>`
			default:
				return `<span class="badge badge-skip">SKIP</span>`
			}
		},
		"kindBadge": func(diff string) template.HTML {
			class := "badge-modified"
			switch diff {
			case "added":
				class = "badge-added"
			case "removed":
				class = "badge-removed"
			}
			return template.HTML(fmt.Sprintf(
				`<span class="badge %s">%s</span>`, class, diff,
			))
		},
	}).Parse(auditEmailTemplate),
)

type EmailAuditSender struct {
	host       string
	port       int
	username   string
	password   string
	tlsMode    string
	from       string
	to         []string
	subjectFmt string
}

func NewEmailAuditSender(
	host string,
	port int,
	username, password string,
	tlsMode string,
	from string,
	to []string,
	subjectFmt string,
) *EmailAuditSender {
	if subjectFmt == "" {
		subjectFmt = "Lexicore Audit Report — %s"
	}
	return &EmailAuditSender{
		host:       host,
		port:       port,
		username:   username,
		password:   password,
		tlsMode:    tlsMode,
		from:       from,
		to:         to,
		subjectFmt: subjectFmt,
	}
}

func (s *EmailAuditSender) Send(targetName string, entries []AuditEntry) error {
	body, err := s.renderHTML(targetName, entries)
	if err != nil {
		return fmt.Errorf("failed to render audit email: %w", err)
	}

	subject := fmt.Sprintf(s.subjectFmt, targetName)
	msg := s.buildMessage(subject, body)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	return s.send(addr, msg)
}

func (s *EmailAuditSender) renderHTML(targetName string, entries []AuditEntry) (string, error) {
	counts := auditCounts{Total: len(entries)}
	emailEntries := make([]auditEmailEntry, 0, len(entries))

	for _, e := range entries {
		switch e.Action {
		case ActionUpdate:
			counts.Updated++
		default:
			counts.Skipped++
		}
		if e.Error != nil {
			counts.Errors++
		}

		ee := auditEmailEntry{
			TimestampFmt: e.Timestamp.Format("2006-01-02 15:04:05 UTC"),
			Action:       e.Action,
			UID:          e.UID,
			Name:         e.Name,
		}
		if e.Error != nil {
			ee.ErrorStr = e.Error.Error()
		}
		for _, c := range e.Changes {
			ee.Changes = append(ee.Changes, auditEmailChange{
				Diff:  changeDiff(c),
				Field: c.Field,
				Old:   c.Old,
				New:   c.New,
			})
		}
		emailEntries = append(emailEntries, ee)
	}

	data := auditEmailData{
		TargetName:  targetName,
		GeneratedAt: time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		ReportID:    fmt.Sprintf("%s-%d", targetName, time.Now().Unix()),
		Counts:      counts,
		Entries:     emailEntries,
	}

	var buf bytes.Buffer
	if err := emailTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *EmailAuditSender) buildMessage(subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString(fmt.Sprintf("From: %s\r\n", s.from))
	b.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(s.to, ", ")))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}

func (s *EmailAuditSender) auth() smtp.Auth {
	if s.username == "" {
		return nil
	}
	return smtp.PlainAuth("", s.username, s.password, s.host)
}

func (s *EmailAuditSender) send(addr string, msg []byte) error {
	switch s.tlsMode {
	case "tls":
		return s.sendImplicitTLS(addr, msg)
	case "starttls":
		return s.sendSTARTTLS(addr, msg)
	default:
		return smtp.SendMail(addr, s.auth(), s.from, s.to, msg)
	}
}

func (s *EmailAuditSender) sendImplicitTLS(addr string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	return s.sendWithClient(conn, msg)
}

func (s *EmailAuditSender) sendSTARTTLS(addr string, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	return s.finishSend(client, msg)
}

func (s *EmailAuditSender) sendWithClient(conn *tls.Conn, msg []byte) error {
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()
	return s.finishSend(client, msg)
}

func (s *EmailAuditSender) finishSend(client *smtp.Client, msg []byte) error {
	if a := s.auth(); a != nil {
		if err := client.Auth(a); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("smtp MAIL: %w", err)
	}
	for _, rcpt := range s.to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	return w.Close()
}
