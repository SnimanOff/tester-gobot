package archive

import (
	"archive/zip"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

var testLimits = Limits{
	MaxArchiveBytes:     20 * 1024 * 1024,
	MaxUnpackedBytes:    10 * 1024 * 1024,
	MaxFiles:            10,
	MaxCompressionRatio: 100,
}

const simpleMainSource = "package main\n\nfunc main() {}\n"

func writeTestArchive(test *testing.T, archiveFiles map[string][]byte) string {
	test.Helper()
	archivePath := filepath.Join(test.TempDir(), "bot.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		test.Fatal(err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	for fileName, fileContent := range archiveFiles {
		entryWriter, err := zipWriter.Create(fileName)
		if err != nil {
			test.Fatal(err)
		}
		if _, err := entryWriter.Write(fileContent); err != nil {
			test.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		test.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		test.Fatal(err)
	}
	return archivePath
}

func randomBytes(byteCount int) []byte {
	randomSource := rand.New(rand.NewSource(1))
	generatedBytes := make([]byte, byteCount)
	randomSource.Read(generatedBytes)
	return generatedBytes
}

func requireRejected(test *testing.T, inspectError error) {
	test.Helper()
	var rejectedError *RejectedError
	if !errors.As(inspectError, &rejectedError) {
		test.Fatalf("expected rejection, got: %v", inspectError)
	}
	test.Logf("rejection reason: %s", rejectedError.Reason)
}

func TestValidArchiveIsExtracted(test *testing.T) {
	archivePath := writeTestArchive(test, map[string][]byte{
		"main.go":            []byte(simpleMainSource),
		"go.mod":             []byte("module childbot\n\ngo 1.22\n"),
		"handlers/handle.go": []byte("package handlers\n"),
	})
	if err := Inspect(archivePath, testLimits); err != nil {
		test.Fatalf("valid archive rejected: %v", err)
	}

	destinationDirectory := filepath.Join(test.TempDir(), "child")
	if err := Extract(archivePath, destinationDirectory, testLimits); err != nil {
		test.Fatalf("extract: %v", err)
	}
	extractedSource, err := os.ReadFile(filepath.Join(destinationDirectory, "main.go"))
	if err != nil || string(extractedSource) != simpleMainSource {
		test.Fatalf("main.go extracted wrong: %q %v", extractedSource, err)
	}
	if _, err := os.Stat(filepath.Join(destinationDirectory, "handlers", "handle.go")); err != nil {
		test.Fatalf("nested folder not extracted: %v", err)
	}
}

func TestMainFileMustBeInRoot(test *testing.T) {
	archivePath := writeTestArchive(test, map[string][]byte{
		"bot/main.go": []byte(simpleMainSource),
	})
	requireRejected(test, Inspect(archivePath, testLimits))
}

func TestPathTraversalIsRejected(test *testing.T) {
	archivePath := writeTestArchive(test, map[string][]byte{
		"main.go":       []byte(simpleMainSource),
		"../outside.go": []byte("package outside\n"),
	})
	requireRejected(test, Inspect(archivePath, testLimits))
	requireRejected(test, Extract(archivePath, filepath.Join(test.TempDir(), "child"), testLimits))
}

func TestTooManyFilesIsRejected(test *testing.T) {
	archiveFiles := map[string][]byte{"main.go": []byte(simpleMainSource)}
	for _, fileName := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10"} {
		archiveFiles[fileName+".txt"] = []byte(fileName)
	}
	requireRejected(test, Inspect(writeTestArchive(test, archiveFiles), testLimits))
}

func TestHighCompressionRatioIsRejected(test *testing.T) {
	archivePath := writeTestArchive(test, map[string][]byte{
		"main.go":   []byte(simpleMainSource),
		"zeros.bin": make([]byte, 5*1024*1024),
	})
	requireRejected(test, Inspect(archivePath, testLimits))
}

func TestTooLargeUnpackedSizeIsRejected(test *testing.T) {
	archivePath := writeTestArchive(test, map[string][]byte{
		"main.go":   []byte(simpleMainSource),
		"noise.bin": randomBytes(11 * 1024 * 1024),
	})
	requireRejected(test, Inspect(archivePath, testLimits))
}

func TestNestedArchiveIsRejected(test *testing.T) {
	archivePath := writeTestArchive(test, map[string][]byte{
		"main.go":  []byte(simpleMainSource),
		"data.zip": []byte("PK"),
	})
	requireRejected(test, Inspect(archivePath, testLimits))
}

func TestNotZipIsRejected(test *testing.T) {
	notZipPath := filepath.Join(test.TempDir(), "bot.zip")
	if err := os.WriteFile(notZipPath, []byte("not an archive"), 0o644); err != nil {
		test.Fatal(err)
	}
	requireRejected(test, Inspect(notZipPath, testLimits))
}

func TestExtractCountsRealBytes(test *testing.T) {
	archivePath := writeTestArchive(test, map[string][]byte{
		"main.go":   []byte(simpleMainSource),
		"noise.bin": randomBytes(64 * 1024),
	})
	smallLimits := testLimits
	smallLimits.MaxUnpackedBytes = 32 * 1024
	requireRejected(test, Extract(archivePath, filepath.Join(test.TempDir(), "child"), smallLimits))
}

func TestCleanEntryName(test *testing.T) {
	for _, unsafeName := range []string{"/etc/passwd", "C:/Windows/evil.go", "..\\evil.go", "dir/../../evil.go"} {
		if _, err := cleanEntryName(unsafeName); err == nil {
			test.Errorf("unsafe path %q passed", unsafeName)
		}
	}
	cleanedName, err := cleanEntryName("dir\\sub\\file.go")
	if err != nil || cleanedName != "dir/sub/file.go" {
		test.Errorf("backslash path: %q %v", cleanedName, err)
	}
}
