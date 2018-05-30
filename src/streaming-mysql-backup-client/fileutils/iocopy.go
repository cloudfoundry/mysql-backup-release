// Copied directly from
// https://github.com/cloudfoundry/cli/blob/a66251ed5c21c649b5d64e793555ff25026ac049/fileutils/iocopy.go
package fileutils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

func CopyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	err = out.Close()
	if err != nil {
		return err
	}

	fileInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		err = os.Chmod(dst, fileInfo.Mode())
		if err != nil {
			return err
		}
	}

	return nil
}

func ExtractFileFields(src string) (map[string]string, error) {

	var keyValMap map[string]string

	in, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	keyValMap = make(map[string]string)

	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		fileLine := scanner.Text()
		if strings.Contains(fileLine, "=") {
			keyValLine := strings.SplitN(fileLine, "=", 2)
			key := strings.TrimSpace(keyValLine[0])
			val := strings.TrimSpace(keyValLine[1])
			keyValMap[key] = val
		}
	}

	return keyValMap, nil
}

func WriteLineToFile(dst, line string) error {
	dstFile, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.WriteString(fmt.Sprintf("%s\n", line))

	return err
}
