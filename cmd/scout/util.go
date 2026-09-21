package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func readPassword() (string, error) {
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func copyFile(dst, src *os.File) (int64, error) { return io.Copy(dst, src) }
