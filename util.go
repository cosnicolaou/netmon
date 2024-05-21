package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func renameIfExisting(name string) error {
	f, err := os.Open(name)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	f.Close()
	date := time.Now().Format(time.RFC3339)
	fmt.Printf("renaming %v to %v\n", name, name+"."+date)
	return os.Rename(name, name+"."+date)
}

func newLogfile(name string) (io.WriteCloser, error) {
	if len(name) == 0 || name == "-" {
		return os.Stdout, nil
	}
	if err := renameIfExisting(name); err != nil {
		return nil, err
	}
	os.MkdirAll(filepath.Dir(name), 0700)
	return os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
}
