package archive

import (
	"archive/zip"
	"fmt"
	"os"
	"path"
	"strings"
)

type Limits struct {
	MaxArchiveBytes     int64
	MaxUnpackedBytes    int64
	MaxFiles            int
	MaxCompressionRatio int64
}

const EntryFileName = "main.go"

const ratioCheckMinBytes = 1024 * 1024

const encryptedEntryFlag = 0x1

var nestedArchiveExtensions = map[string]bool{
	".zip": true, ".rar": true, ".7z": true, ".tar": true, ".gz": true,
	".tgz": true, ".bz2": true, ".xz": true, ".zst": true,
}

type RejectedError struct {
	Reason string
}

func (rejectedError *RejectedError) Error() string {
	return rejectedError.Reason
}

func reject(reasonFormat string, reasonArguments ...any) error {
	return &RejectedError{Reason: fmt.Sprintf(reasonFormat, reasonArguments...)}
}

func Inspect(archivePath string, limits Limits) error {
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return err
	}
	if archiveInfo.Size() > limits.MaxArchiveBytes {
		return reject("архив больше %d МБ", limits.MaxArchiveBytes/(1024*1024))
	}

	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return reject("файл не является zip-архивом или поврежден")
	}
	defer zipReader.Close()

	if len(zipReader.File) > limits.MaxFiles {
		return reject("в архиве %d файлов, допустимо не больше %d", len(zipReader.File), limits.MaxFiles)
	}

	var declaredUnpackedBytes uint64
	seenEntryNames := make(map[string]bool, len(zipReader.File))
	hasEntryFile := false

	for _, archiveEntry := range zipReader.File {
		entryName, err := cleanEntryName(archiveEntry.Name)
		if err != nil {
			return err
		}
		lowerEntryName := strings.ToLower(entryName)
		if seenEntryNames[lowerEntryName] {
			return reject("файл %q встречается в архиве дважды", entryName)
		}
		seenEntryNames[lowerEntryName] = true

		entryMode := archiveEntry.Mode()
		if entryMode&os.ModeSymlink != 0 {
			return reject("в архиве есть символическая ссылка %q", entryName)
		}
		if !entryMode.IsDir() && !entryMode.IsRegular() {
			return reject("в архиве есть специальный файл %q", entryName)
		}
		if archiveEntry.Flags&encryptedEntryFlag != 0 {
			return reject("зашифрованные архивы не поддерживаются")
		}
		if entryMode.IsDir() {
			continue
		}

		if nestedArchiveExtensions[strings.ToLower(path.Ext(entryName))] {
			return reject("вложенные архивы запрещены: %q", entryName)
		}
		if archiveEntry.UncompressedSize64 > uint64(limits.MaxUnpackedBytes)-declaredUnpackedBytes {
			return reject("после распаковки архив больше %d МБ - похоже на zip-бомбу", limits.MaxUnpackedBytes/(1024*1024))
		}
		declaredUnpackedBytes += archiveEntry.UncompressedSize64
		if isCompressionRatioTooHigh(archiveEntry, limits.MaxCompressionRatio) {
			return reject("файл %q сжат больше чем в %d раз - похоже на zip-бомбу", entryName, limits.MaxCompressionRatio)
		}

		if entryName == EntryFileName {
			hasEntryFile = true
		}
	}

	if !hasEntryFile {
		return reject("в корне архива нет файла %s", EntryFileName)
	}
	return nil
}

func cleanEntryName(rawEntryName string) (string, error) {
	slashedName := strings.ReplaceAll(rawEntryName, "\\", "/")
	if strings.HasPrefix(slashedName, "/") || strings.Contains(slashedName, ":") {
		return "", reject("в архиве есть абсолютный путь %q", rawEntryName)
	}
	cleanedName := path.Clean(slashedName)
	if cleanedName == ".." || strings.HasPrefix(cleanedName, "../") {
		return "", reject("в архиве есть путь за пределы папки: %q", rawEntryName)
	}
	return cleanedName, nil
}

func isCompressionRatioTooHigh(archiveEntry *zip.File, maxCompressionRatio int64) bool {
	if archiveEntry.UncompressedSize64 < ratioCheckMinBytes {
		return false
	}
	if archiveEntry.CompressedSize64 == 0 {
		return true
	}
	return archiveEntry.UncompressedSize64/archiveEntry.CompressedSize64 > uint64(maxCompressionRatio)
}
