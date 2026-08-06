package notify

import (
	"bytes"
	"html/template"
)

// emailTheme is the shared visual design for every outbound email this
// platform sends. auth-service (password reset / email verification) keeps
// its own copy of the same HTML/CSS -- the two Go modules can't share a
// package, so this is kept in sync by hand across both when the design
// changes. table-based layout + inline styles since mail clients (Outlook
// especially) don't reliably support external/embedded stylesheets or flex.
const emailTheme = `<!doctype html>
<html>
  <body style="margin:0;padding:32px 16px;background:#f1f4f6;font-family:-apple-system,'Segoe UI',Roboto,Arial,sans-serif;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:480px;margin:0 auto;background:#ffffff;border-radius:12px;overflow:hidden;border:1px solid #dce3e7;">
      <tr><td style="background:#10202c;padding:20px 28px;">
        <span style="color:#ffffff;font-size:16px;font-weight:700;letter-spacing:.02em;">YGATE Solar SCADA</span>
      </td></tr>
      <tr><td style="padding:28px;">
        <h1 style="margin:0 0 12px;font-size:18px;line-height:1.4;color:#0f1b26;">{{.Title}}</h1>
        {{if .Body}}<p style="margin:0 0 20px;font-size:14px;line-height:1.6;color:#4a5a66;white-space:pre-line;">{{.Body}}</p>{{end}}
        {{if .ButtonURL}}<a href="{{.ButtonURL}}" style="display:inline-block;background:#0e5c73;color:#ffffff;text-decoration:none;font-size:14px;font-weight:700;padding:12px 24px;border-radius:8px;">{{.ButtonLabel}}</a>{{end}}
        {{if .Rows}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-top:4px;border-collapse:collapse;">{{range .Rows}}<tr><td style="padding:7px 0;font-size:13px;color:#4a5a66;border-bottom:1px solid #eef1f2;">{{.Key}}</td><td style="padding:7px 0;font-size:13px;color:#0f1b26;font-weight:600;text-align:right;border-bottom:1px solid #eef1f2;">{{.Value}}</td></tr>{{end}}</table>{{end}}
      </td></tr>
      <tr><td style="padding:16px 28px;background:#f7f9fa;border-top:1px solid #dce3e7;">
        <p style="margin:0;font-size:11px;color:#8a97a0;">This is an automated message from YGATE Solar SCADA. Please do not reply to this email.</p>
      </td></tr>
    </table>
  </body>
</html>`

var parsedEmailTheme = template.Must(template.New("email").Parse(emailTheme))

// EmailRow is one label/value line in an email's detail table (e.g. Plant,
// Device, Severity for an alarm breach).
type EmailRow struct{ Key, Value string }

// EmailContent is the themed template's input: Title/Body are always shown,
// ButtonURL+ButtonLabel render a CTA button when both are set, Rows render
// a detail table when non-empty. All fields are HTML-escaped by html/template.
type EmailContent struct {
	Title, Body            string
	ButtonURL, ButtonLabel string
	Rows                   []EmailRow
}

// RenderEmail applies EmailContent to the shared theme, producing ready-to-send HTML.
func RenderEmail(content EmailContent) (string, error) {
	var buf bytes.Buffer
	if err := parsedEmailTheme.Execute(&buf, content); err != nil {
		return "", err
	}
	return buf.String(), nil
}
