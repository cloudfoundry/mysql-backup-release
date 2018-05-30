package fileutils

import (
	"errors"
	"io"
	"os"
)

type StreamingFileWriter struct {
	Filename string
}

func (sfw StreamingFileWriter) WriteStream(reader io.Reader) error {
	file, err := os.Create(sfw.Filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return errors.New("Error writing reader contents to file")
	}

	return nil
}
