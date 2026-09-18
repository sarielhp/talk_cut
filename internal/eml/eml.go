// Package eml discovers and parses seminar announcement email (.eml) files.
package eml

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ParsedEmail represents the extracted fields from an email announcement.
type ParsedEmail struct {
	Subject string
	From    string
	Date    string
	Body    string
}

var tagRegex = regexp.MustCompile(`<[^>]*>`)

// FindUniqueEML finds the single .eml file in dir.
// Returns empty string if no .eml file is found.
// Returns an error if multiple .eml files are found (ambiguous).
func FindUniqueEML(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading directory %q for .eml files: %w", dir, err)
	}

	var emlFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".eml") {
			emlFiles = append(emlFiles, filepath.Join(dir, e.Name()))
		}
	}

	if len(emlFiles) == 0 {
		return "", nil
	}
	if len(emlFiles) > 1 {
		return "", fmt.Errorf("multiple .eml files found in %q (%d files): ambiguous, expected a unique .eml file", dir, len(emlFiles))
	}
	return emlFiles[0], nil
}

// ParseEMLFile reads and parses an RFC 822 / MIME .eml file into ParsedEmail.
func ParseEMLFile(path string) (*ParsedEmail, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening eml file %q: %w", path, err)
	}
	defer f.Close()

	msg, err := mail.ReadMessage(f)
	if err != nil {
		return nil, fmt.Errorf("parsing eml message %q: %w", path, err)
	}

	sub := msg.Header.Get("Subject")
	from := msg.Header.Get("From")
	date := msg.Header.Get("Date")
	contentType := msg.Header.Get("Content-Type")
	encoding := msg.Header.Get("Content-Transfer-Encoding")

	body, err := extractBody(msg.Body, contentType, encoding)
	if err != nil {
		return nil, fmt.Errorf("extracting eml body: %w", err)
	}

	return &ParsedEmail{
		Subject: strings.TrimSpace(sub),
		From:    strings.TrimSpace(from),
		Date:    strings.TrimSpace(date),
		Body:    strings.TrimSpace(body),
	}, nil
}

// extractBody handles both single-part and multipart MIME message bodies.
func extractBody(r io.Reader, contentType, encoding string) (string, error) {
	if contentType == "" {
		contentType = "text/plain"
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary, ok := params["boundary"]
		if !ok {
			return "", fmt.Errorf("multipart missing boundary")
		}
		return extractMultipart(r, boundary)
	}

	return readDecodedPart(r, mediaType, encoding)
}

// extractMultipart recursively scans multipart parts for plain text or html.
func extractMultipart(r io.Reader, boundary string) (string, error) {
	mr := multipart.NewReader(r, boundary)
	var plainBuf, htmlBuf strings.Builder

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading multipart: %w", err)
		}

		pType, pParams, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		pEnc := part.Header.Get("Content-Transfer-Encoding")

		if strings.HasPrefix(pType, "multipart/") {
			if subBoundary, ok := pParams["boundary"]; ok {
				if subText, subErr := extractMultipart(part, subBoundary); subErr == nil && subText != "" {
					return subText, nil
				}
			}
			continue
		}

		text, readErr := readDecodedPart(part, pType, pEnc)
		if readErr != nil {
			continue
		}

		if pType == "text/plain" && strings.TrimSpace(text) != "" {
			plainBuf.WriteString(text)
			plainBuf.WriteString("\n")
		} else if pType == "text/html" && strings.TrimSpace(text) != "" {
			htmlBuf.WriteString(stripHTML(text))
			htmlBuf.WriteString("\n")
		}
	}

	if plainBuf.Len() > 0 {
		return plainBuf.String(), nil
	}
	return htmlBuf.String(), nil
}

// readDecodedPart decodes transfer encoding (quoted-printable or base64) and returns text.
func readDecodedPart(r io.Reader, mediaType, encoding string) (string, error) {
	var decodedReader = r
	encLower := strings.ToLower(strings.TrimSpace(encoding))

	switch encLower {
	case "quoted-printable":
		decodedReader = quotedprintable.NewReader(r)
	case "base64":
		decodedReader = base64.NewDecoder(base64.StdEncoding, r)
	}

	data, err := io.ReadAll(decodedReader)
	if err != nil {
		return "", fmt.Errorf("reading decoded content: %w", err)
	}

	res := string(data)
	if mediaType == "text/html" {
		res = stripHTML(res)
	}
	return res, nil
}

// stripHTML removes HTML tags and converts basic entities.
func stripHTML(html string) string {
	text := tagRegex.ReplaceAllString(html, " ")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	return strings.Join(strings.Fields(text), " ")
}
