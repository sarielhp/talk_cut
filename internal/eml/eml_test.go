package eml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindUniqueEML(t *testing.T) {
	tempDir := t.TempDir()

	// 0 EML files
	path, err := FindUniqueEML(tempDir)
	if err != nil {
		t.Fatalf("unexpected error on empty dir: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}

	// 1 EML file
	eml1 := filepath.Join(tempDir, "talk.eml")
	err = os.WriteFile(eml1, []byte("From: test\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	path, err = FindUniqueEML(tempDir)
	if err != nil {
		t.Fatalf("unexpected error on 1 eml: %v", err)
	}
	if path != eml1 {
		t.Errorf("expected %q, got %q", eml1, path)
	}

	// 2 EML files -> should error
	eml2 := filepath.Join(tempDir, "other.eml")
	err = os.WriteFile(eml2, []byte("From: test2\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = FindUniqueEML(tempDir)
	if err == nil {
		t.Fatal("expected error on multiple eml files, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected 'ambiguous' in error, got %v", err)
	}
}

func TestParseEMLFile(t *testing.T) {
	tempDir := t.TempDir()
	sampleEML := `From: Boris Aronov <boris@example.com>
Subject: [Seminar] Michiel Smid Talk
Date: Tue, 8 Sep 2026 22:43:38 +0200
Content-Type: multipart/alternative; boundary="boundary42"

--boundary42
Content-Type: text/plain; charset="UTF-8"
Content-Transfer-Encoding: quoted-printable

Oriented Spanners in Metric Spaces

Speaker: Michiel Smid, Carleton University

Abstract:
A t-spanner of a finite metric space is an undirected graph.
We prove bounds on (1+=CE=B5)-spanners.
--boundary42
Content-Type: text/html; charset="UTF-8"

<p>HTML version</p>
--boundary42--
`

	emlPath := filepath.Join(tempDir, "test.eml")
	if err := os.WriteFile(emlPath, []byte(sampleEML), 0644); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseEMLFile(emlPath)
	if err != nil {
		t.Fatalf("ParseEMLFile failed: %v", err)
	}

	if parsed.Subject != "[Seminar] Michiel Smid Talk" {
		t.Errorf("unexpected subject: %q", parsed.Subject)
	}
	if !strings.Contains(parsed.From, "Boris Aronov") {
		t.Errorf("unexpected from: %q", parsed.From)
	}
	if !strings.Contains(parsed.Body, "Oriented Spanners in Metric Spaces") {
		t.Errorf("expected body to contain title, got: %q", parsed.Body)
	}
	if !strings.Contains(parsed.Body, "(1+ε)-spanners") {
		t.Errorf("expected body to decode quoted-printable epsilon, got: %q", parsed.Body)
	}
}

func TestParseRealEML(t *testing.T) {
	realPath := "../../examples/26/09/15/announcement.eml"
	if _, err := os.Stat(realPath); err != nil {
		t.Skip("real announcement.eml not found")
	}

	parsed, err := ParseEMLFile(realPath)
	if err != nil {
		t.Fatalf("ParseEMLFile on real EML failed: %v", err)
	}

	if !strings.Contains(parsed.Body, "Oriented Spanners in Metric Spaces") {
		t.Errorf("expected real EML body to contain title, got: %q", parsed.Body)
	}
	if !strings.Contains(parsed.Body, "Michiel Smid") {
		t.Errorf("expected real EML body to contain speaker, got: %q", parsed.Body)
	}

	t.Logf("Parsed Subject: %s", parsed.Subject)
	t.Logf("Parsed Body preview:\n%s\n", parsed.Body[:500])
}
