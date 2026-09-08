// demo-install downloads exactly the Runner binary pinned by this repository.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type pin struct{ version, file, sha string }

func readPin(raw, platform string) (pin, error) {
	var p pin
	section, artifact := "", ""
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			section = strings.TrimSuffix(trimmed, ":")
			artifact = ""
			continue
		}
		if section != "runner" {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if key == "version" {
			p.version = value
		}
		if indent == 8 && value == "" {
			artifact = key
		}
		if artifact == platform {
			switch key {
			case "file":
				p.file = value
			case "sha256":
				p.sha = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return p, err
	}
	validVersion := regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	validSHA := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if !validVersion.MatchString(p.version) || !validSHA.MatchString(p.sha) || p.file != "runner_"+p.version+"_"+strings.ReplaceAll(platform, "-", "_") {
		return p, errors.New("invalid or missing Runner pin for " + platform)
	}
	return p, nil
}

func install(ctx context.Context, client *http.Client, base, destination string, p pin) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+p.version+"/"+p.file, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("release download returned HTTP %d", response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(destination), ".runner-download-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(response.Body, (128<<20)+1))
	if err != nil {
		return err
	}
	if size > 128<<20 || fmt.Sprintf("%x", hash.Sum(nil)) != p.sha {
		return errors.New("Runner checksum does not match toolchain.lock")
	}
	if err := f.Chmod(0700); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), destination)
}

func main() {
	lock := flag.String("lock", ".local-ci/toolchain.lock", "repository toolchain lock")
	destination := flag.String("output", ".runner-ci/bin/runner", "binary destination")
	flag.Parse()
	raw, err := os.ReadFile(*lock)
	var p pin
	if err == nil {
		p, err = readPin(string(raw), runtime.GOOS+"-"+runtime.GOARCH)
	}
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		err = install(ctx, http.DefaultClient, "https://github.com/cajoy/runner-dist/releases/download", *destination, p)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Installed Runner %s at %s; checksum matches toolchain.lock\n", p.version, *destination)
}
