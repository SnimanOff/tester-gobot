package archive

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Extract(archivePath, destinationDirectory string, limits Limits) error {
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return reject("файл не является zip-архивом или поврежден")
	}
	defer zipReader.Close()

	absoluteDestination, err := filepath.Abs(destinationDirectory)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absoluteDestination, 0o755); err != nil {
		return err
	}

	remainingBytes := limits.MaxUnpackedBytes
	for _, archiveEntry := range zipReader.File {
		entryName, err := cleanEntryName(archiveEntry.Name)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(absoluteDestination, filepath.FromSlash(entryName))
		relativePath, err := filepath.Rel(absoluteDestination, targetPath)
		if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return reject("в архиве есть путь за пределы папки: %q", archiveEntry.Name)
		}

		if archiveEntry.Mode().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		writtenBytes, err := extractFile(archiveEntry, targetPath, remainingBytes)
		if err != nil {
			return err
		}
		remainingBytes -= writtenBytes
	}
	return nil
}

func extractFile(archiveEntry *zip.File, targetPath string, remainingBytes int64) (int64, error) {
	entryReader, err := archiveEntry.Open()
	if err != nil {
		return 0, reject("не удалось прочитать %q из архива", archiveEntry.Name)
	}
	defer entryReader.Close()

	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer targetFile.Close()

	writtenBytes, err := io.Copy(targetFile, io.LimitReader(entryReader, remainingBytes+1))
	if writtenBytes > remainingBytes {
		return writtenBytes, reject("распакованные файлы больше допустимого размера - похоже на zip-бомбу")
	}
	if err != nil {
		return writtenBytes, reject("архив поврежден: %q не распаковывается", archiveEntry.Name)
	}
	return writtenBytes, nil
}
